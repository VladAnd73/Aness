package fleet

import (
	"errors"
	"path/filepath"

	"github.com/versality/spore/internal/task"
)

// ErrLoopBusy is a drive that found another drive of the same slug
// mid-pass. The systemd reconcile timer and the coordinator's Stop
// hook can overlap; without the lock both would run the same command
// check and action.
var ErrLoopBusy = errors.New("another drive of this loop is running")

// lockLoopDrive takes the slug's non-blocking drive lock.
func lockLoopDrive(projectRoot, slug string) (func(), bool) {
	return flockBeside(filepath.Join(task.RoleTaskDir(projectRoot, slug), "state", "drive"))
}
