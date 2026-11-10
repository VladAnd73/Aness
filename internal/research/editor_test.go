package research

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/evalharness"
	"github.com/versality/spore/internal/isolate"
)

func requireBwrap(t *testing.T) {
	t.Helper()
	if err := isolate.Available(); err != nil {
		t.Skipf("bwrap cannot run here: %v", err)
	}
}

func testCreds(t *testing.T) {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".credentials.json")
	body, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "at-test", "refreshToken": "rt-test", "expiresAt": time.Now().Add(10 * time.Hour).UnixMilli(),
	}})
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUDGET_CREDS", p)
}

func editorScratch(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"SKILL.md": "# demo\n", "goal.md": "be better\n", "results.tsv": "try\tscore\n", "frozen/ref.md": "frozen\n"} {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// capture records the command an editor would run and answers with fn.
func capture(cmds *[]*exec.Cmd, fn func(*exec.Cmd)) func(*exec.Cmd) (string, int, error) {
	return func(c *exec.Cmd) (string, int, error) {
		*cmds = append(*cmds, c)
		if fn != nil {
			fn(c)
		}
		return `{"type":"result","result":"ok"}`, 0, nil
	}
}

func claudeArgv(t *testing.T, c *exec.Cmd) []string {
	t.Helper()
	i := slices.Index(c.Args, "--")
	if i < 0 {
		t.Fatalf("no bwrap -- in %v", c.Args)
	}
	return c.Args[i+1:]
}

func TestClaudeEditorArgs(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	var cmds []*exec.Cmd
	scratch := editorScratch(t)
	ed := ClaudeEditor{Binary: trueBin(t), Model: "haiku", run: capture(&cmds, nil)}
	if _, err := ed.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md", Try: 1}); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("ran %d commands", len(cmds))
	}
	got := claudeArgv(t, cmds[0])
	want := []string{trueBin(t), "-p", "--tools", "Read,Edit,Write", "--permission-mode", "acceptEdits",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "project",
		"--output-format", "stream-json", "--verbose", "--model", "haiku", "--"}
	if len(got) != len(want)+1 || !slices.Equal(got[:len(want)], want) {
		t.Fatalf("claude argv =\n%q\nwant\n%q + prompt", got, want)
	}
	if prompt := got[len(got)-1]; !strings.Contains(prompt, "SKILL.md") || !strings.Contains(prompt, ".try-description") || !strings.Contains(prompt, "goal.md") {
		t.Errorf("prompt does not name the target, goal and description file: %q", prompt)
	}
	if slices.Contains(got, "Bash") || strings.Contains(strings.Join(got, " "), "Bash") {
		t.Errorf("editor gets Bash: %v", got)
	}
	noModel := ClaudeEditor{Binary: trueBin(t), run: capture(&cmds, nil)}
	noModel.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md"})
	if len(cmds) < 2 || slices.Contains(cmds[1].Args, "--model") {
		t.Errorf("empty Model still passes --model: %v", cmds[1].Args)
	}
}

func bindTriples(args []string) [][3]string {
	var out [][3]string
	for i := 0; i+2 < len(args) && args[i] != "--"; i++ {
		switch args[i] {
		case "--bind", "--ro-bind", "--ro-bind-try", "--ro-bind-data":
			out = append(out, [3]string{args[i], args[i+1], args[i+2]})
			i += 2
		}
	}
	return out
}

func TestClaudeEditorSpecBindsOnlyScratch(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	var cmds []*exec.Cmd
	scratch := editorScratch(t)
	ed := ClaudeEditor{Binary: trueBin(t), run: capture(&cmds, nil)}
	if _, err := ed.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md"}); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("ran %d commands", len(cmds))
	}
	hostHome, _ := os.UserHomeDir()
	frozen := filepath.Join(scratch, "frozen")
	sawScratch, sawFrozen, sawCreds := false, false, false
	for _, b := range bindTriples(cmds[0].Args) {
		flag, src, dst := b[0], b[1], b[2]
		switch {
		case flag == "--bind" && src == scratch && dst == scratch:
			sawScratch = true
		case flag == "--ro-bind" && src == frozen && dst == frozen:
			sawFrozen = true
		case flag == "--ro-bind-data" && strings.HasSuffix(dst, "/.claude/.credentials.json"):
			sawCreds = true
		case flag == "--ro-bind-data" && strings.HasPrefix(dst, "/nix/store/"), flag == "--ro-bind-data" && dst == "/etc/hosts":
		case flag != "--bind" && slices.Contains([]string{"/nix", "/etc", "/run/current-system", "/bin", "/usr/bin"}, src):
		case flag == "--bind" && src == dst && !strings.HasPrefix(src, scratch) && !strings.HasPrefix(src, hostHome+"/"):
			// the editor's private home
		default:
			t.Errorf("unexpected bind %v", b)
		}
	}
	if !sawScratch || !sawFrozen || !sawCreds {
		t.Errorf("scratch rw %v, frozen ro %v, credentials ro %v; want all three", sawScratch, sawFrozen, sawCreds)
	}
}

func TestClaudeEditorReadsDescription(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	var cmds []*exec.Cmd
	scratch := editorScratch(t)
	write := func(c *exec.Cmd) {
		os.WriteFile(filepath.Join(scratch, ".try-description"), []byte("  Tighten the opening "+strings.Repeat("x", 300)+"\nsecond line\n"), 0o600)
	}
	res, err := ClaudeEditor{Binary: trueBin(t), run: capture(&cmds, write)}.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Description, "Tighten the opening x") || len(res.Description) != 200 {
		t.Errorf("description = %q (%d bytes), want the first line trimmed to 200 bytes", res.Description, len(res.Description))
	}
	if res.Transcript != `{"type":"result","result":"ok"}` {
		t.Errorf("transcript = %q", res.Transcript)
	}
	if _, err := os.Stat(filepath.Join(scratch, ".try-description")); err == nil {
		t.Error(".try-description left in the scratch dir")
	}
}

func TestClaudeEditorMissingDescription(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	var cmds []*exec.Cmd
	res, err := ClaudeEditor{Binary: trueBin(t), run: capture(&cmds, nil)}.Propose(context.Background(), EditRequest{ScratchDir: editorScratch(t), TargetRel: "SKILL.md"})
	if err != nil || res.Description != "(no description)" {
		t.Errorf("Propose = %+v, %v; want (no description)", res, err)
	}
}

func TestClaudeEditorRefusesUnsandboxedWithoutOptIn(t *testing.T) {
	t.Setenv(evalharness.AllowUnsandboxedEnv, "")
	var cmds []*exec.Cmd
	_, err := ClaudeEditor{Isolation: "none", Binary: trueBin(t), run: capture(&cmds, nil)}.Propose(context.Background(), EditRequest{ScratchDir: editorScratch(t), TargetRel: "SKILL.md"})
	if err == nil || !strings.Contains(err.Error(), evalharness.AllowUnsandboxedEnv) || len(cmds) != 0 {
		t.Errorf("err = %v, ran %d; want a refusal naming %s", err, len(cmds), evalharness.AllowUnsandboxedEnv)
	}
}

func TestClaudeEditorFrozenIsReadOnlyInsideBwrap(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	scratch := editorScratch(t)
	agent := filepath.Join(scratch, "fake-editor")
	script := "#!/bin/sh\necho changed >> frozen/ref.md || echo FROZEN-RO\necho edited >> SKILL.md\necho 'appended a line' > .try-description\n"
	if err := os.WriteFile(agent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	res, err := ClaudeEditor{Binary: agent}.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md"})
	if err != nil {
		t.Fatalf("Propose: %v (%s)", err, res.Transcript)
	}
	if b, _ := os.ReadFile(filepath.Join(scratch, "frozen", "ref.md")); string(b) != "frozen\n" {
		t.Errorf("frozen file changed: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(scratch, "SKILL.md")); string(b) != "# demo\nedited\n" {
		t.Errorf("target edit did not land: %q", b)
	}
	if res.Description != "appended a line" || !strings.Contains(res.Transcript, "FROZEN-RO") {
		t.Errorf("result = %+v", res)
	}
}

func trueBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no true on PATH: %v", err)
	}
	return p
}

// The live acceptance run shows claude's permission layer refusing an
// outside read; this proves the sandbox hides the file on its own.
func TestClaudeEditorSandboxHidesOutsideFiles(t *testing.T) {
	requireBwrap(t)
	testCreds(t)
	planted := filepath.Join(t.TempDir(), "store", "planted.txt")
	os.MkdirAll(filepath.Dir(planted), 0o700)
	if err := os.WriteFile(planted, []byte("PLANTED-TOKEN\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scratch := editorScratch(t)
	agent := filepath.Join(scratch, "fake-editor")
	script := "#!/bin/sh\ncat " + planted + " || echo STORE-HIDDEN\nls \"$HOME\"/.. >/dev/null\n"
	if err := os.WriteFile(agent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	res, err := ClaudeEditor{Binary: agent}.Propose(context.Background(), EditRequest{ScratchDir: scratch, TargetRel: "SKILL.md"})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if strings.Contains(res.Transcript, "PLANTED-TOKEN") || !strings.Contains(res.Transcript, "STORE-HIDDEN") {
		t.Errorf("editor sandbox read the planted file:\n%s", res.Transcript)
	}
}
