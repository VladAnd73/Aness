package fleet

import (
	"context"
	"time"
)

// RoleWatcher tracks role-loop state for one task. Each Tick polls
// `.spore/<slug>/` via DeriveSnapshot, returns the current state, and
// reports whether anything moved since the previous Tick. Terminal
// phases (done, escalated) are surfaced once and remain stable
// afterwards.
//
// The watcher is intentionally polling-only and stateless beyond the
// previous snapshot: callers drive the cadence (a goroutine sleeping
// between ticks in production, a test driving Tick directly). The
// coordinator binds the emitted snapshot to side effects (spawn role
// pane, kill A pane, alert operator).
type RoleWatcher struct {
	ProjectRoot string
	Slug        string

	last Snapshot
	seen bool
}

// NewRoleWatcher returns a watcher for the given task. No I/O
// happens until Tick.
func NewRoleWatcher(projectRoot, slug string) *RoleWatcher {
	return &RoleWatcher{ProjectRoot: projectRoot, Slug: slug}
}

// Tick reads the current snapshot. The bool is true the first time
// (so the caller bootstraps from the initial state) and true on any
// later tick where the snapshot changed from the previous one.
// Returns the latest error from DeriveSnapshot.
func (w *RoleWatcher) Tick() (Snapshot, bool, error) {
	snap, err := DeriveSnapshot(w.ProjectRoot, w.Slug)
	if err != nil {
		return Snapshot{}, false, err
	}
	changed := !w.seen || snap != w.last
	w.last = snap
	w.seen = true
	return snap, changed, nil
}

// Terminal reports whether the watcher has settled into a phase that
// will not advance on its own (done or escalated). Useful as a loop
// break in callers driving Tick on a sleep cadence.
func (w *RoleWatcher) Terminal() bool {
	if !w.seen {
		return false
	}
	return w.last.Phase == PhaseDone || w.last.Phase == PhaseEscalated
}

// Run drives Tick at interval until ctx is cancelled or the watcher
// reaches a terminal phase. Each changed snapshot is delivered on the
// returned channel. The channel closes on exit. Errors are surfaced
// via the error return only on the first failure: a fatal Derive
// error stops the loop.
func (w *RoleWatcher) Run(ctx context.Context, interval time.Duration) (<-chan Snapshot, <-chan error) {
	out := make(chan Snapshot)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		t := time.NewTicker(interval)
		defer t.Stop()
		// Emit the bootstrap snapshot before the first tick wait so
		// callers see the initial state immediately.
		snap, changed, err := w.Tick()
		if err != nil {
			errs <- err
			return
		}
		if changed {
			select {
			case <-ctx.Done():
				return
			case out <- snap:
			}
		}
		if w.Terminal() {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				snap, changed, err := w.Tick()
				if err != nil {
					errs <- err
					return
				}
				if changed {
					select {
					case <-ctx.Done():
						return
					case out <- snap:
					}
				}
				if w.Terminal() {
					return
				}
			}
		}
	}()
	return out, errs
}
