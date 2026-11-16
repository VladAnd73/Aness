package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// loopNoteFakeAgent stands in for claude: it learns where to write and
// which author to claim only from its first prompt, the way a real
// agent reads the step note ahead of a role body that names other
// paths.
const loopNoteFakeAgent = `#!/bin/sh
prompt="$*"
out=$(printf '%s\n' "$prompt" | grep -o '/[^ ]*/round-{round}\.json' | head -n1 | sed 's/{round}/1/')
if [ -z "$out" ]; then
  printf '%s: no output path in prompt\n' "$SPORE_STEP" >> "$SPORE_PROJECT_ROOT/agent.log"
  exec sleep 30
fi
mkdir -p "$(dirname "$out")"
case "$SPORE_STEP" in
  check)
    author=$(printf '%s\n' "$prompt" | grep -o '"author": "[^"]*"' | head -n1 | sed 's/.*: "//; s/"$//')
    printf '{"author":"%s","verdict":"approve","summary":"ok","comments":[]}' "$author" > "$out" ;;
  *)
    printf '{"summary":"ok"}' > "$out" ;;
esac
exec sleep 30
`

// TestLoopStepPromptCarriesOutputPath: given the bundled example loop
// and agents that follow only their first prompt, when Reconcile
// ticks, then each step writes where the loop looks and the loop
// reaches done. Before the step note, the bundled role bodies pointed
// panes at the legacy role-loop paths and the loop stalled at draft.
func TestLoopStepPromptCarriesOutputPath(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	bin := t.TempDir()
	agent := filepath.Join(bin, "fake-agent")
	if err := os.WriteFile(agent, []byte(loopNoteFakeAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dirs.project, "agent.log")

	fm := "---\nstatus: active\nslug: ex\nagent: " + agent + "\nloop: example\n---\nsmoke brief\n"
	if err := os.WriteFile(filepath.Join(dirs.tasks, "ex.md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		killSporeSessions(dirs.project)
		killRoleSessions(dirs.project)
	})

	taskDir := task.RoleTaskDir(dirs.project, "ex")
	ready := filepath.Join(taskDir, "state", "ready")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3}); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if b, err := os.ReadFile(log); err == nil {
		t.Errorf("fake agent could not act on its prompt:\n%s", b)
	}
	for _, rel := range []string{"draft/round-1.json", "check/round-1.json"} {
		if _, err := os.Stat(filepath.Join(taskDir, rel)); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}
	if _, err := os.Stat(ready); err != nil {
		t.Errorf("example loop never reached done (no state/ready): %v", err)
	}
}

// TestStepNoteNamesPathShapeAndAuthor: given the example loop's two
// agent steps, when their panes are specced, then the step note names
// the absolute output template, the keys the shape requires, the
// author a verdict must carry, and that it overrides the role body.
func TestStepNoteNamesPathShapeAndAuthor(t *testing.T) {
	root := newGitRepoFor(t, "note-project")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	def, err := LoadLoop(root, "example")
	if err != nil {
		t.Fatal(err)
	}
	dir := task.RoleTaskDir(root, "ex")
	cases := []struct {
		step string
		want []string
	}{
		{"draft", []string{filepath.Join(dir, "draft", "round-{round}.json"), `"summary"`, "role instructions"}},
		{"check", []string{filepath.Join(dir, "check", "round-{round}.json"), `"author": "check"`, `"approve"`, `"request_changes"`, "role instructions"}},
	}
	for _, c := range cases {
		args, err := StepSpec(root, "ex", def, def.Steps[def.StepIndex(c.step)]).TmuxArgs()
		if err != nil {
			t.Fatalf("%s: TmuxArgs: %v", c.step, err)
		}
		note := ""
		for i, a := range args {
			if i > 0 && args[i-1] == "-e" && strings.HasPrefix(a, StepNoteEnv+"=") {
				note = strings.TrimPrefix(a, StepNoteEnv+"=")
			}
		}
		if note == "" {
			t.Errorf("%s: no %s in tmux args %q", c.step, StepNoteEnv, args)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(note, w) {
				t.Errorf("%s: step note lacks %q:\n%s", c.step, w, note)
			}
		}
		if got := args[len(args)-1]; !strings.Contains(got, StepNoteEnv) {
			t.Errorf("%s: shell command never reads %s: %s", c.step, StepNoteEnv, got)
		}
	}
}
