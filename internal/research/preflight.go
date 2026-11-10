package research

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// preflight refuses a night that must not start, in the spec's order,
// and returns the held lock and the weekly usage at night start.
func preflight(ctx context.Context, s Store, cfg Config, d Deps) (unlock func(), startUsed float64, err error) {
	refuse := func(format string, a ...any) (func(), float64, error) {
		return nil, 0, &RefusedError{Reason: fmt.Sprintf(format, a...)}
	}
	if cfg.Status != StatusOn {
		return refuse("campaign %s is %s, not on", cfg.Name, cfg.Status)
	}
	release, holder, err := Lock(d.LockPath, cfg.Name)
	if err != nil {
		if holder != "" {
			return refuse("another night holds the lock: %s", holder)
		}
		return nil, 0, err
	}
	ok := false
	defer func() {
		if !ok {
			release()
		}
	}()
	if d.Room == nil {
		return refuse("budget room said: no room check configured")
	}
	if code, line := d.Room(); code != 0 {
		return refuse("budget room said: %s", strings.TrimSpace(line))
	}
	dir := s.Dir(cfg.Name)
	for _, h := range []struct{ name, want, path string }{
		{"main", cfg.MainHash, filepath.Join(dir, "judge", "main")},
		{"holdout", cfg.HoldoutHash, filepath.Join(dir, "judge", "holdout")},
		{"binary", cfg.BinaryHash, filepath.Join(dir, "bin", "spore")},
	} {
		hash := HashFile
		if h.name != "binary" {
			hash = HashTree
		}
		if got, err := hash(h.path); err != nil || got != h.want {
			return refuse("judge hash mismatch: %s", h.name)
		}
	}
	if clean, err := s.repo(cfg.Name).Clean(); err != nil {
		return nil, 0, err
	} else if !clean {
		return refuse("campaign repo has uncommitted changes")
	}
	if goal, err := os.ReadFile(cfg.GoalFile); err != nil || strings.TrimSpace(string(goal)) == "" {
		return refuse("goal file %s is missing or empty", cfg.GoalFile)
	}
	if d.WeeklyUsed == nil {
		return refuse("quota-read-failed: no weekly usage reader configured")
	}
	used, err := d.WeeklyUsed(ctx)
	if err != nil {
		return refuse("quota-read-failed: %v", err)
	}
	ok = true
	return release, used, nil
}
