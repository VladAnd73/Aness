package task

import (
	"os"
	"path/filepath"
	"testing"
)

// Given a finished loop task's tree under .spore/<slug>/ (spec cache,
// verdicts, ready marker), when the task is closed with Done, then
// the tree goes with it, so a later task reusing the slug starts its
// loop from round 1 with its own spec. Other slugs' trees stay.
func TestDoneRemovesTheLoopTaskTree(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "x.md"), []byte("---\nstatus: active\nslug: x\nloop: dev\n---\nold brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_EVIDENCE_WARN_ONLY", "1")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, slug := range []string{"x", "other"} {
		if err := WriteSpec(root, slug, []byte("old brief\n")); err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(RoleTaskDir(root, slug), "state")
		if err := os.MkdirAll(state, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, "ready"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Done(tasksDir, "x", true); err != nil {
		t.Fatalf("Done: %v", err)
	}
	if _, err := os.Stat(RoleTaskDir(root, "x")); !os.IsNotExist(err) {
		t.Errorf(".spore/x survives Done (stat err %v): a reused slug would inherit its spec and verdicts", err)
	}
	if SpecExists(root, "other") == false {
		t.Errorf(".spore/other was removed by Done of x")
	}
}

// Given a slug that climbs out of .spore/ ("../tasks/x" resolves to the
// real task file tasks/x.md), when Done runs, then it removes nothing
// outside .spore/: here the directory tasks/x/ beside the task file.
func TestDoneRemovesNothingOutsideTheLoopTree(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, "tasks")
	keep := filepath.Join(tasksDir, "x", "notes.txt")
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "x.md"), []byte("---\nstatus: active\nslug: x\n---\nbrief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_EVIDENCE_WARN_ONLY", "1")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := Done(tasksDir, "../tasks/x", true); err != nil {
		t.Fatalf("Done: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("Done of slug %q removed %s outside .spore/: %v", "../tasks/x", keep, err)
	}
}
