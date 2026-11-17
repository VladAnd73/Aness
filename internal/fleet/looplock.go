package fleet

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/versality/spore/internal/task"
)

// ErrLoopBusy is a drive that found another drive of the same slug
// mid-pass. The systemd reconcile timer and the coordinator's Stop
// hook can overlap; without the lock both would run the same command
// check and action.
var ErrLoopBusy = errors.New("another drive of this loop is running")

// lockLoopDrive takes the slug's non-blocking drive lock. Only a lock
// held by another drive is ErrLoopBusy; any other failure is returned
// as is, since Reconcile does not print ErrLoopBusy.
func lockLoopDrive(projectRoot, slug string) (func(), error) {
	if err := task.EnsureRoleTaskRoot(projectRoot); err != nil {
		return nil, fmt.Errorf("drive lock: %w", err)
	}
	unlock, err := tryFlock(filepath.Join(task.RoleTaskDir(projectRoot, slug), "state", "drive.lock"))
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, ErrLoopBusy
	}
	if err != nil {
		return nil, fmt.Errorf("drive lock: %w", err)
	}
	return unlock, nil
}
