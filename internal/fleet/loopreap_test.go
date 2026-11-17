package fleet

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// Given loop panes of a done task, a deleted task and a paused task,
// plus a pane of the same slug in another project, when Reconcile runs,
// then the done and deleted tasks' panes are reaped and the paused
// task's and the other project's are kept, as generic workers are.
func TestReconcileReapsLoopPanesOfFinishedTasks(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	writeTask(t, dirs.tasks, "finished", "done")
	writeTask(t, dirs.tasks, "held", "paused")
	project, err := task.ProjectName(dirs.project)
	if err != nil {
		t.Fatal(err)
	}
	other := "other-" + project
	sessions := map[string]bool{
		fmt.Sprintf("spore-role/%s/finished/draft", project): false,
		fmt.Sprintf("spore-role/%s/gone/check", project):     false,
		fmt.Sprintf("spore-role/%s/held/draft", project):     true,
		fmt.Sprintf("spore-role/%s/finished/draft", other):   true,
	}
	for s := range sessions {
		if out, err := exec.Command("tmux", "-L", testTmuxSocket, "new-session", "-d", "-s", s, "sleep 60").CombinedOutput(); err != nil {
			t.Fatalf("tmux new-session %s: %v: %s", s, err, out)
		}
	}
	t.Cleanup(func() {
		for s := range sessions {
			_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", "="+s).Run()
		}
		killSporeSessions(dirs.project)
	})

	if _, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	for s, keep := range sessions {
		if got := hasSession(s); got != keep {
			t.Errorf("%s: alive = %v, want %v", s, got, keep)
		}
	}
}

// Given a loop pane of a done task spawned moments ago, when Reconcile
// reaps it, then its spawn-backoff record goes with it, as reapRolePane
// does: role-spawns.json is never pruned otherwise, and a task reopened
// within the quick-death window would wait out a backoff for a planned
// kill.
func TestReconcileLoopPaneReapForgetsSpawnRecord(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	writeTask(t, dirs.tasks, "finished", "done")
	project, err := task.ProjectName(dirs.project)
	if err != nil {
		t.Fatal(err)
	}
	s := fmt.Sprintf("spore-role/%s/finished/draft", project)
	if out, err := exec.Command("tmux", "-L", testTmuxSocket, "new-session", "-d", "-s", s, "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session %s: %v: %s", s, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", "="+s).Run()
		killSporeSessions(dirs.project)
	})
	g := loadRoleSpawnBackoff(dirs.project)
	g.spawned("finished/draft", time.Now())
	if err := g.save(); err != nil {
		t.Fatal(err)
	}

	if _, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if hasSession(s) {
		t.Fatalf("%s: still alive, want reaped", s)
	}
	if r := loadRoleSpawnBackoff(dirs.project).recs["finished/draft"]; r != nil {
		t.Errorf("spawn record of the reaped pane survives: %+v", *r)
	}
}
