package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeSporeToml(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPreStart(t *testing.T) {
	root := t.TempDir()
	if got := LoadPreStart(root); got != (PreStart{TimeoutSeconds: DefaultPreStartTimeoutSeconds}) {
		t.Errorf("missing spore.toml: got %+v", got)
	}
	writeSporeToml(t, root, "[fleet]\npre_start = \"  bin/with-services up  \"\npre_start_timeout_seconds = 0\n")
	if got := LoadPreStart(root); got != (PreStart{Cmd: "bin/with-services up"}) {
		t.Errorf("got %+v, want trimmed command, unbounded", got)
	}
	writeSporeToml(t, root, "[fleet]\npre_start = \"x\"\npre_start_timeout_seconds = nope\n")
	if got := LoadPreStart(root).TimeoutSeconds; got != DefaultPreStartTimeoutSeconds {
		t.Errorf("bad timeout: got %d, want default", got)
	}
}

func TestWorkerShellCommandUnsetIsAgent(t *testing.T) {
	if got := workerShellCommand(PreStart{}.Prelude("/p", "s", "worker"), "claude-code"); got != "claude-code" {
		t.Errorf("unset pre_start changed the agent command: %q", got)
	}
}

func TestWorkerShellCommandWrapsAgent(t *testing.T) {
	cmd := `echo 'it''s' && true`
	got := workerShellCommand(PreStart{Cmd: cmd, TimeoutSeconds: 900}.Prelude("/p", "feat", "worker"), "claude-code")
	for _, want := range []string{
		"mkdir -p /p/.spore/feat/state && : > /p/.spore/feat/state/pre-start-worker.running && " +
			"flock /p/.spore/feat/state/pre-start.lock timeout 900 sh -c ",
		"rm -f /p/.spore/feat/state/pre-start-worker.running; ",
		shellQuote(cmd),
		`exec claude-code ${spore_pre_start_note:+"$spore_pre_start_note"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func requireFlock(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock not on PATH")
	}
}

func TestWorkerShellCommandRuns(t *testing.T) {
	requireFlock(t)
	cases := []struct {
		name, cmd, want string
	}{
		{"success", `printf "%s" 'it'"'"'s' > ran`, ""},
		{"failure", `echo 'quote'"'"'d' >/dev/null; exit 3`, `spore: pre-start hook failed (exit 3): echo 'quote'"'"'d' >/dev/null; exit 3. Services may be down.`},
		{"timeout", `test -f "$SPORE_MARKER" && sleep 5`, `spore: pre-start hook failed (exit 124): test -f "$SPORE_MARKER" && sleep 5. Services may be down.`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			marker := PreStartMarkerPath(root, "feat", "worker")
			script := workerShellCommand(PreStart{Cmd: c.cmd, TimeoutSeconds: 1}.Prelude(root, "feat", "worker"), "printf %s")
			cmd := exec.Command("sh", "-c", script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "SPORE_MARKER="+marker)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v: %s", err, out)
			}
			if string(out) != c.want {
				t.Errorf("agent got %q, want %q", out, c.want)
			}
			if c.name == "success" {
				if b, err := os.ReadFile(filepath.Join(root, "ran")); err != nil || string(b) != "it's" {
					t.Errorf("pre_start did not run in cwd: %q, %v", b, err)
				}
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("marker left behind after the hook: %v", err)
			}
			if _, err := os.Stat(PreStartLockPath(root, "feat")); err != nil {
				t.Errorf("lock file: %v", err)
			}
		})
	}
}
