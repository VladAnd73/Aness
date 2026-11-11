package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Bug 1: the coordinator's Stop hook fires in whatever cwd the session
// is in. From inside a worktree, the worktree's frozen copy of a brief
// still says active although the real task is paused, and the hook
// used to respawn it.
func TestReplenishHookFromWorktreeDoesNotRespawnPausedTask(t *testing.T) {
	requireToolchain(t)
	bin := sharedSporeBin(t)
	root := gitInitProject(t)
	project := filepath.Base(root)
	killProjectSessions(t, project)

	writeFile(t, filepath.Join(root, "tasks", "w2.md"), taskFile("w2", "paused"), 0o644)
	wt := filepath.Join(root, ".worktrees", "w2")
	gitIn(t, root, "worktree", "add", "-q", wt, "-b", "wt/w2")
	writeFile(t, filepath.Join(wt, "tasks", "w2.md"), taskFile("w2", "active"), 0o644)

	env := fleetEnv(t, "sleep 300")
	coordState := filepath.Join(t.TempDir(), "coordinator")
	env = append(env,
		"SPORE_TASK_SLUG=coordinator",
		"SPORE_COORDINATOR_STATE_DIR="+coordState,
		"SPORE_TASK_INBOX="+filepath.Join(coordState, project, "inbox"),
		"SPORE_PROJECT_ROOT="+root,
	)

	res := runCLI(t, bin, wt, env, "{}", "fleet", "replenish-hook")
	if res.code != 0 {
		t.Fatalf("replenish-hook exit %d: %s", res.code, res.stderr)
	}
	if strings.Contains(res.stdout, "spawned=1") || hasTmuxSession(t, "spore/"+project+"/w2") {
		t.Fatalf("paused task w2 was respawned from the worktree's brief copy:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "active=0") {
		t.Fatalf("hook did not read the main tasks/ dir (want active=0):\n%s%s", res.stdout, res.stderr)
	}
}
