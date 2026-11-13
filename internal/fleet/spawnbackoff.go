package fleet

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/versality/spore/internal/task"
)

// Reconcile cannot tell a worker that crashed from one that finished: it
// sees an active task with no session either way. A worker found dead
// within quickDeathWindow of its last spawn counts as a failed spawn, and
// each failure in a row doubles the wait before the next attempt.
const (
	quickDeathWindow = 10 * time.Minute
	spawnBackoffBase = time.Minute
	spawnBackoffCap  = 30 * time.Minute
)

type spawnRecord struct {
	Last time.Time `json:"last"`
	// Streak is the number of spawns in a row that died at once or
	// within quickDeathWindow.
	Streak int `json:"streak"`
	// Counted is set once the death of the spawn at Last is in Streak,
	// so a pass that only waits out the backoff does not count it again.
	Counted bool `json:"counted"`
}

type spawnBackoff struct {
	path string
	recs map[string]*spawnRecord
}

func loadSpawnBackoff(projectRoot string) *spawnBackoff {
	b := &spawnBackoff{recs: map[string]*spawnRecord{}}
	dir, err := task.StateDirForProject(projectRoot)
	if err != nil {
		return b
	}
	b.path = filepath.Join(dir, "fleet-spawns.json")
	raw, err := os.ReadFile(b.path)
	if err != nil {
		return b
	}
	_ = json.Unmarshal(raw, &b.recs)
	if b.recs == nil {
		b.recs = map[string]*spawnRecord{}
	}
	return b
}

// wait reports how long slug must still wait before a spawn, counting
// the death of its previous spawn first if this pass is the first to
// see it.
func (b *spawnBackoff) wait(slug string, now time.Time) time.Duration {
	r := b.recs[slug]
	if r == nil {
		return 0
	}
	if !r.Counted {
		if now.Sub(r.Last) < quickDeathWindow {
			r.Streak++
		} else {
			r.Streak = 0
		}
		r.Counted = true
	}
	if r.Streak == 0 {
		return 0
	}
	d := spawnBackoffBase
	for i := 1; i < r.Streak && d < spawnBackoffCap; i++ {
		d *= 2
	}
	if d > spawnBackoffCap {
		d = spawnBackoffCap
	}
	return r.Last.Add(d).Sub(now)
}

func (b *spawnBackoff) spawned(slug string, now time.Time) {
	r := b.recs[slug]
	if r == nil {
		r = &spawnRecord{}
		b.recs[slug] = r
	}
	r.Last = now
	r.Counted = false
}

func (b *spawnBackoff) failed(slug string, now time.Time) {
	b.spawned(slug, now)
	r := b.recs[slug]
	r.Streak++
	r.Counted = true
}

// prune forgets slug once its worker has outlived quickDeathWindow, and
// drops records of slugs no longer active.
func (b *spawnBackoff) prune(running, active map[string]bool, now time.Time) {
	for slug, r := range b.recs {
		if !active[slug] || (running[slug] && now.Sub(r.Last) >= quickDeathWindow) {
			delete(b.recs, slug)
		}
	}
}

func (b *spawnBackoff) save() error {
	if b.path == "" {
		return nil
	}
	if len(b.recs) == 0 {
		if err := os.Remove(b.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	raw, err := json.MarshalIndent(b.recs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}
