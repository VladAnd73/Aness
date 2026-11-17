package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/versality/spore/internal/task"
)

// Given another drive holds a slug's loop, when a second drive of the
// same slug runs, then it skips the slug: no pane spawns, no action
// runs, and it reports ErrLoopBusy. Once the first drive lets go, the
// next drive runs the action exactly once.
func TestConcurrentLoopDriveSkipsTheBusySlug(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	f.tick(t)
	f.write(t, "a/round-1.json", `{"result":"x"}`)
	f.tick(t)
	f.write(t, "b/round-1.json", `{"result":"y"}`)
	spawns := len(f.panes.spawns)

	unlock, err := lockLoopDrive(f.root, f.slug)
	if err != nil {
		t.Fatalf("could not take the drive lock: %v", err)
	}
	if _, err := DriveLoop(f.root, f.tasks, f.slug); !errors.Is(err, ErrLoopBusy) {
		t.Errorf("drive under a held lock: err = %v, want ErrLoopBusy", err)
	}
	if n := countContaining(inboxBodies(t, f.inbox), "trio reached c"); n != 0 {
		t.Errorf("action ran %d times while another drive held the slug, want 0", n)
	}
	if len(f.panes.spawns) != spawns {
		t.Errorf("spawns %v while another drive held the slug", f.panes.spawns[spawns:])
	}
	unlock()

	f.tick(t)
	f.tick(t)
	if n := countContaining(inboxBodies(t, f.inbox), "trio reached c"); n != 1 {
		t.Errorf("action ran %d times after the lock was released, want 1", n)
	}
}

// Given the drive lock cannot be opened at all (here the state
// directory is a plain file), when a drive runs, then it reports that
// error. ErrLoopBusy is not it: Reconcile hides ErrLoopBusy, so the
// loop would stall every tick with nothing on stderr.
func TestDriveLockFailureIsNotReportedAsBusy(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	f.tick(t)
	state := filepath.Join(task.RoleTaskDir(f.root, f.slug), "state")
	if err := os.RemoveAll(state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DriveLoop(f.root, f.tasks, f.slug)
	if err == nil || errors.Is(err, ErrLoopBusy) {
		t.Errorf("drive with an unopenable lock: err = %v, want a non-busy error", err)
	}
}
