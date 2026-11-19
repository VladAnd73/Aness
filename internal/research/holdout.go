package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Stop ends a campaign for reason. With a kept change it first runs the
// sealed holdout set on the original and the winner. It takes the night
// locks, host-wide included, since the holdout spends quota.
func Stop(ctx context.Context, s Store, name, reason string, d Deps) error {
	d = d.withDefaults()
	h, err := s.lockLoad(name, lockOp{what: "stop", scope: hostNight, host: d.LockPath, check: func(cfg Config) error {
		switch cfg.Status {
		case StatusReady, StatusOn, StatusOff:
			return nil
		}
		return fmt.Errorf("research: campaign %s is %s; only a ready, on or off campaign can be stopped", name, cfg.Status)
	}})
	var busy *BusyError
	if errors.As(err, &busy) {
		return &RefusedError{Reason: busy.Reason()}
	}
	if err != nil {
		return err
	}
	defer h.release()
	h.releaseConfig()
	return stopCampaign(ctx, s, h.cfg, reason, d)
}

func (cfg Config) hasKeptChange() bool {
	return cfg.BestCommit != "" && cfg.BestCommit != cfg.StartCommit
}

// holdoutPending is the engine's own switch-off of a campaign with a
// kept change whose stop never finished: the night was interrupted, or
// the holdout failed. Only stop may end it.
func (cfg Config) holdoutPending() bool {
	return cfg.Status == StatusOff && cfg.hasKeptChange() && (cfg.StopReason == "no-gain" || cfg.StopReason == "max-nights")
}

// stopCampaign runs with the night locks held, so best and counters
// cannot move under it; only an off can, and stopped replaces it. A
// holdout failure leaves the status untouched, so the stop can be
// retried.
func stopCampaign(ctx context.Context, s Store, cfg Config, reason string, d Deps) error {
	var h holdoutResult
	if cfg.hasKeptChange() {
		var err error
		if h, err = spentHoldout(ctx, s, cfg, d.Judge); err != nil {
			return err
		}
	}
	lk, err := s.lockLoad(cfg.Name, lockOp{})
	if err != nil {
		return err
	}
	cfg = lk.cfg
	if cfg.hasKeptChange() {
		cfg.JudgeCostUSD += h.cost
		cfg.HoldoutVerdict = "overfit"
		if roundScore(h.winner) >= roundScore(h.original) {
			cfg.HoldoutVerdict = "pass"
		}
	}
	cfg.Status, cfg.StopReason = StatusStopped, reason
	err = s.Save(cfg)
	lk.release()
	if err != nil {
		return err
	}
	report, err := finalReport(s, cfg, h)
	if err != nil {
		return err
	}
	if err := appendReport(s, cfg.Name, report); err != nil {
		return err
	}
	verdict := cfg.HoldoutVerdict
	if verdict == "" {
		verdict = "no change"
	}
	msg := fmt.Sprintf("research %s stopped (%s): main %.1f -> %.1f, holdout %s; see %s",
		cfg.Name, reason, cfg.StartScore, cfg.BestScore, verdict, filepath.Join(s.Dir(cfg.Name), "report.md"))
	if d.Notify == nil {
		return appendReport(s, cfg.Name, "Warning: no coordinator notice configured.\n\n")
	}
	if err := d.Notify(msg); err != nil {
		return appendReport(s, cfg.Name, fmt.Sprintf("Warning: the coordinator notice failed: %v\n\n", err))
	}
	return nil
}

type holdoutResult struct{ original, winner, cost float64 }

// heldoutFile records a finished holdout, so a stop that failed after it
// reuses the result instead of paying for it again.
type heldoutFile struct {
	Start, Best               string
	Original, Winner, CostUSD float64
}

func spentHoldout(ctx context.Context, s Store, cfg Config, j Judge) (holdoutResult, error) {
	path := filepath.Join(s.Dir(cfg.Name), "holdout", "result.json")
	var f heldoutFile
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &f) == nil && f.Start == cfg.StartCommit && f.Best == cfg.BestCommit {
		return holdoutResult{original: f.Original, winner: f.Winner, cost: f.CostUSD}, nil
	}
	h, err := runHoldout(ctx, s, cfg, j)
	if err != nil {
		return h, err
	}
	b, err := json.Marshal(heldoutFile{Start: cfg.StartCommit, Best: cfg.BestCommit, Original: h.original, Winner: h.winner, CostUSD: h.cost})
	if err != nil {
		return h, err
	}
	return h, os.WriteFile(path, b, 0o600)
}

func runHoldout(ctx context.Context, s Store, cfg Config, j Judge) (holdoutResult, error) {
	if j == nil {
		return holdoutResult{}, errors.New("research: stop: no judge configured")
	}
	var h holdoutResult
	r := s.repo(cfg.Name)
	dir := filepath.Join(s.Dir(cfg.Name), "holdout")
	for _, side := range []struct {
		name, commit string
		score        *float64
	}{{"original", cfg.StartCommit, &h.original}, {"winner", cfg.BestCommit, &h.winner}} {
		body, err := r.show(side.commit, cfg.targetRel())
		if err != nil {
			return h, err
		}
		cand := filepath.Join(dir, side.name, cfg.targetRel())
		if err := os.MkdirAll(filepath.Dir(cand), 0o700); err != nil {
			return h, err
		}
		if err := os.WriteFile(cand, body, 0o600); err != nil {
			return h, err
		}
		var total float64
		for i := 1; i <= 2; i++ {
			res, err := j.Run(ctx, cand, "holdout", filepath.Join(dir, fmt.Sprintf("%s-%d.json", side.name, i)))
			if err != nil {
				return h, fmt.Errorf("research: holdout on the %s: %w", side.name, err)
			}
			h.cost += res.CostUSD
			total += res.Score
		}
		*side.score = total / 2
	}
	return h, nil
}
