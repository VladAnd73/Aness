package fleet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDriveLoopKeepsSporeOutOfGitStatus: a fresh repo with no ignore
// rules shows nothing under .spore/ once a loop task has started, so
// a done check such as `git status --porcelain` is not tripped.
func TestDriveLoopKeepsSporeOutOfGitStatus(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	f.tick(t)
	f.write(t, "alpha/round-1.json", `{"notes":"1"}`)
	if _, err := os.Stat(loopTaskDir(f.root, f.slug)); err != nil {
		t.Fatalf("loop created no task tree: %v", err)
	}
	out, err := exec.Command("git", "-C", f.root, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, out)
	}
	if strings.Contains(string(out), ".spore/") {
		t.Fatalf("git status shows loop state:\n%s", out)
	}
}

// TestDriveLoopSurvivesUnwritableExclude: an info/exclude spore cannot
// write (read-only, another owner) must not stop the drive.
func TestDriveLoopSurvivesUnwritableExclude(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	info := filepath.Join(f.root, ".git", "info")
	if err := os.Chmod(filepath.Join(info, "exclude"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(info, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(info, 0o755) })
	if _, err := DriveLoop(f.root, f.tasks, f.slug); err != nil {
		t.Fatalf("DriveLoop: %v", err)
	}
}

// TestDriveLoopExcludesOnlyWhenSporeIsCreated: once .spore/ exists a
// drive tick does no exclude work, so an exclude line the operator
// removed stays removed.
func TestDriveLoopExcludesOnlyWhenSporeIsCreated(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	f.tick(t)
	exclude := filepath.Join(f.root, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.tick(t)
	if b, _ := os.ReadFile(exclude); len(b) != 0 {
		t.Fatalf("a later tick rewrote info/exclude: %q", b)
	}
}
