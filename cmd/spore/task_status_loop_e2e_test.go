package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Given tasks on the example loop, the dev loop and no loop, when
// `spore task status` runs, then it reads each task's own loop: the
// example task reports its current step, the dev task keeps the
// role-loop lines, and a task without a loop is refused.
func TestTaskStatusFollowsTheTasksLoop(t *testing.T) {
	bin := sharedSporeBin(t)
	root := t.TempDir()
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	task := func(slug, loop string) {
		fm := "---\nstatus: active\nslug: " + slug + "\n"
		if loop != "" {
			fm += "loop: " + loop + "\n"
		}
		writeFile(t, filepath.Join(root, "tasks", slug+".md"), fm+"---\nbrief\n", 0o644)
	}
	task("ex", "example")
	task("dv", "dev")
	task("plain", "")
	writeFile(t, filepath.Join(root, ".spore", "ex", "draft", "round-1.json"), `{"summary":"x"}`, 0o644)
	env := e2eEnv(t)

	res := runCLI(t, bin, root, env, "", "task", "status", "ex")
	if res.code != 0 {
		t.Fatalf("status ex: exit %d: %s", res.code, res.stderr)
	}
	for _, want := range []string{"loop:     example", "step:     check"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("status ex lacks %q:\n%s", want, res.stdout)
		}
	}

	res = runCLI(t, bin, root, env, "", "task", "status", "dv")
	if res.code != 0 {
		t.Fatalf("status dv: exit %d: %s", res.code, res.stderr)
	}
	for _, want := range []string{"loop:     dev", "phase:    engineer-initial"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("status dv lacks %q:\n%s", want, res.stdout)
		}
	}

	res = runCLI(t, bin, root, env, "", "task", "status", "plain")
	if res.code == 0 || !strings.Contains(res.stderr, "no loop") {
		t.Errorf("status plain: exit %d, stderr %q; want a refusal naming the missing loop", res.code, res.stderr)
	}
}
