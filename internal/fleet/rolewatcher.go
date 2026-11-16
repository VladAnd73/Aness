package fleet

import (
	"context"
	"time"
)

// Compile stub for the configurable-loops acceptance tests. The task
// that ports upstream's internal/fleet/rolewatcher.go replaces this file.

type RoleWatcher struct {
	ProjectRoot string
	Slug        string
}

func NewRoleWatcher(projectRoot, slug string) *RoleWatcher {
	return &RoleWatcher{ProjectRoot: projectRoot, Slug: slug}
}

func (w *RoleWatcher) Tick() (Snapshot, bool, error) {
	return Snapshot{}, false, nil
}

func (w *RoleWatcher) Terminal() bool {
	return false
}

func (w *RoleWatcher) Run(ctx context.Context, interval time.Duration) (<-chan Snapshot, <-chan error) {
	out := make(chan Snapshot)
	errs := make(chan error)
	close(out)
	close(errs)
	return out, errs
}
