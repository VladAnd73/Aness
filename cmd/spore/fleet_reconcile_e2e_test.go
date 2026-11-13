package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func reconcileProject(t *testing.T, agent string) (bin, root, project string, env []string) {
	t.Helper()
	requireToolchain(t)
	bin = sharedSporeBin(t)
	root = gitInitProject(t)
	project = filepath.Base(root)
	killProjectSessions(t, project)
	return bin, root, project, fleetEnv(t, agent)
}

// Bug 5: one line the strict frontmatter parser rejects used to drop the
// task from the list, and reconcile then reaped its live worker as if the
// task were done.
func TestReconcileKeepsWorkerWhoseTaskFileDoesNotParse(t *testing.T) {
	bin, root, project, env := reconcileProject(t, "sleep 300")
	path := filepath.Join(root, "tasks", "gamma.md")
	writeFile(t, path, taskFile("gamma", "active"), 0o644)

	if res := runCLI(t, bin, root, env, "", "fleet", "reconcile"); !strings.Contains(res.stdout, "spawned: gamma") {
		t.Fatalf("setup: gamma not spawned:\n%s%s", res.stdout, res.stderr)
	}
	raw, _ := os.ReadFile(path)
	broken := strings.Replace(string(raw), "title: gamma\n", "title: gamma\n# reviewer: keep this running\n", 1)
	writeFile(t, path, broken, 0o644)

	res := runCLI(t, bin, root, env, "", "fleet", "reconcile")
	if strings.Contains(res.stdout, "reaped: gamma") || !hasTmuxSession(t, "spore/"+project+"/gamma") {
		t.Fatalf("reconcile reaped the live worker of an unparseable task:\n%s%s", res.stdout, res.stderr)
	}
	ls := runCLI(t, bin, root, env, "", "task", "ls")
	if !strings.Contains(ls.stdout+ls.stderr, "gamma") {
		t.Fatalf("task ls hides the unparseable task:\n%s%s", ls.stdout, ls.stderr)
	}
}

// Bug 6: the coordinator pauses a task right after its DONE tell. The
// paused session is kept for the operator, but it must not hold one of
// the max-workers slots, and it must not be respawned.
func TestReconcilePausedSessionDoesNotHoldAWorkerSlot(t *testing.T) {
	bin, root, project, env := reconcileProject(t, "sleep 300")
	env = append(env, "SPORE_FLEET_MAX_WORKERS=1")
	writeFile(t, filepath.Join(root, "tasks", "alpha.md"), taskFile("alpha", "active"), 0o644)
	writeFile(t, filepath.Join(root, "tasks", "beta.md"), taskFile("beta", "draft"), 0o644)

	if res := runCLI(t, bin, root, env, "", "fleet", "reconcile"); !strings.Contains(res.stdout, "spawned: alpha") {
		t.Fatalf("setup: alpha not spawned:\n%s%s", res.stdout, res.stderr)
	}
	if res := runCLI(t, bin, root, env, "", "task", "pause", "alpha"); res.code != 0 {
		t.Fatalf("pause alpha: %s", res.stderr)
	}
	writeFile(t, filepath.Join(root, "tasks", "beta.md"), taskFile("beta", "active"), 0o644)

	res := runCLI(t, bin, root, env, "", "fleet", "reconcile")
	if !strings.Contains(res.stdout, "spawned: beta") {
		t.Fatalf("paused alpha holds the only slot; beta not spawned:\n%s%s", res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "spawned: alpha") {
		t.Fatalf("paused alpha was respawned:\n%s", res.stdout)
	}
	if !hasTmuxSession(t, "spore/"+project+"/alpha") {
		t.Fatal("the paused session was reaped; pause keeps it for the operator")
	}
}

// Bug 7, first half: tmux new-session succeeds even when the agent
// binary does not exist, so a spawn that died at once was reported as
// started.
func TestTaskStartFailsWhenTheAgentDiesAtOnce(t *testing.T) {
	bin, root, _, env := reconcileProject(t, "/nonexistent/agent")
	writeFile(t, filepath.Join(root, "tasks", "nobin.md"), taskFile("nobin", "draft"), 0o644)

	res := runCLI(t, bin, root, env, "", "task", "start", "nobin")
	if res.code == 0 {
		t.Fatalf("task start reported success for an agent that died at once:\n%s%s", res.stdout, res.stderr)
	}
}

// Bug 7, second half: reconcile respawned a dying worker every pass. A
// slug whose spawn failed, or whose worker died soon after a spawn, must
// back off instead.
func TestReconcileBacksOffAWorkerThatKeepsDying(t *testing.T) {
	for _, tc := range []struct{ name, agent string }{
		{"missing binary", "/nonexistent/agent"},
		{"dies after a second", "sleep 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, root, _, env := reconcileProject(t, tc.agent)
			writeFile(t, filepath.Join(root, "tasks", "flaky.md"), taskFile("flaky", "active"), 0o644)

			first := runCLI(t, bin, root, env, "", "fleet", "reconcile")
			time.Sleep(1500 * time.Millisecond)
			for i := 2; i <= 3; i++ {
				res := runCLI(t, bin, root, env, "", "fleet", "reconcile")
				if strings.Contains(res.stdout, "spawned: flaky") {
					t.Fatalf("pass %d respawned flaky with no backoff:\npass 1:\n%s\npass %d:\n%s%s",
						i, first.stdout+first.stderr, i, res.stdout, res.stderr)
				}
				if !strings.Contains(res.stdout+res.stderr, "flaky") {
					t.Fatalf("pass %d does not mention flaky; the backoff must be visible:\n%s%s", i, res.stdout, res.stderr)
				}
			}
		})
	}
}
