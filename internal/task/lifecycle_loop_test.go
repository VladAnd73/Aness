package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task/frontmatter"
)

func TestStartRefusesRoleLoop(t *testing.T) {
	assertStartRefusesLoop(t, "role")
}

func TestStartRefusesAnyLoop(t *testing.T) {
	assertStartRefusesLoop(t, "dev")
}

func assertStartRefusesLoop(t *testing.T, loop string) {
	t.Helper()
	tasksDir := t.TempDir()
	taskPath := filepath.Join(tasksDir, "x.md")
	orig := "---\nstatus: draft\nslug: x\ntitle: X\nloop: " + loop + "\n---\n"
	if err := os.WriteFile(taskPath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := Start(tasksDir, "x")
	if err == nil {
		t.Fatalf("Start on loop: %s task should error, got session %q", loop, session)
	}
	if !strings.Contains(err.Error(), "loop: "+loop) {
		t.Errorf("error %q should name the loop: %s opt-in", err, loop)
	}
	if session != "" {
		t.Errorf("session = %q, want none", session)
	}
	raw, readErr := os.ReadFile(taskPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != orig {
		t.Errorf("task file mutated by refused Start:\n%s", raw)
	}
}

func TestCodexAgentCommandIgnoresAgentBinaryOverride(t *testing.T) {
	t.Setenv(AgentBinaryEnv, "sleep 30")
	got, err := CodexAgentCommand(frontmatter.Meta{
		Agent: "codex",
		Extra: map[string]string{"effort": "very-high", "model": "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("CodexAgentCommand: %v", err)
	}
	want := "codex --dangerously-bypass-approvals-and-sandbox --no-alt-screen --disable apps -m gpt-5.5 -c 'model_reasoning_effort=\"xhigh\"'"
	if got != want {
		t.Errorf("command = %q want %q", got, want)
	}
}

func TestEnsureWorktreeCreatesBranchAndCopiesBrief(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := gitCmd(repo, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	tasksDir := filepath.Join(repo, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	brief := "---\nstatus: active\nslug: x\nloop: dev\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(tasksDir, "x.md"), []byte(brief), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := EnsureWorktree(tasksDir, "x")
	if err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}
	if want := filepath.Join(repo, ".worktrees", "x"); wt != want {
		t.Errorf("worktree = %q want %q", wt, want)
	}
	if !branchExists(repo, "wt/x") {
		t.Error("branch wt/x not created")
	}
	got, err := os.ReadFile(filepath.Join(wt, "tasks", "x.md"))
	if err != nil || string(got) != brief {
		t.Errorf("brief copy = %q, %v", got, err)
	}
	again, err := EnsureWorktree(tasksDir, "x")
	if err != nil || again != wt {
		t.Errorf("second EnsureWorktree = %q, %v; want idempotent %q", again, err, wt)
	}
}
