package research

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// preflight refuses a night that must not start, in the spec's order.
// It takes the night locks and loads the config under them, and returns
// both, with the config lock already released, and the weekly usage at
// night start.
func preflight(ctx context.Context, s Store, name string, d Deps) (*held, float64, error) {
	refuse := func(format string, a ...any) (*held, float64, error) {
		return nil, 0, &RefusedError{Reason: fmt.Sprintf(format, a...)}
	}
	h, err := s.lockLoad(name, lockOp{what: "night", scope: hostNight, host: d.LockPath, check: func(cfg Config) error {
		if cfg.Status != StatusOn {
			return &RefusedError{Reason: fmt.Sprintf("campaign %s is %s, not on", cfg.Name, cfg.Status)}
		}
		return nil
	}})
	if err != nil {
		return nil, 0, err
	}
	h.releaseConfig()
	ok := false
	defer func() {
		if !ok {
			h.release()
		}
	}()
	cfg := h.cfg
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
	return h, used, nil
}
