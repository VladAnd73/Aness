package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// limitReached names the setting whose limit has run out, or "". An
// off campaign with a kept change at a limit can only end by stop.
func (cfg Config) limitReached() string {
	switch {
	case cfg.Nights >= cfg.MaxNights:
		return "max_nights"
	case cfg.NoGainNights >= cfg.StopAfterNightsWithoutGain:
		return "stop_after_nights_without_gain"
	}
	return ""
}

// stopCampaign runs with the night locks held, so best and counters
// cannot move under it; only an off can, and stopped replaces it. A
// holdout failure leaves the status untouched, so the stop can be
// retried.
func stopCampaign(ctx context.Context, s Store, cfg Config, reason string, d Deps) error {
	var h holdoutResult
	if cfg.hasKeptChange() {
		var err error
		if h, err = runHoldout(ctx, s, cfg, d.Judge); err != nil {
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
		cfg.HoldoutVerdict = holdoutVerdict(cfg.Objective, h)
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

// holdoutVerdict passes a maximize-score winner only when it beats the
// original: a tie is no evidence the edit generalises, and an original
// already at 100 has no headroom to show a gain. shrink-holding-score
// wants the same score from a shorter file, so holding the score passes.
func holdoutVerdict(objective string, h holdoutResult) string {
	if objective == "shrink-holding-score" {
		if roundScore(h.winner) >= roundScore(h.original) {
			return "pass"
		}
		return "overfit"
	}
	switch {
	case roundScore(h.original) >= 100:
		return "no-headroom"
	case roundScore(h.winner) > roundScore(h.original):
		return "pass"
	}
	return "overfit"
}

// heldoutSide is one holdout side already paid for, kept in
// holdout/result.json so a stop that fails later reuses it. Judge keys
// it to the judge that scored it.
type heldoutSide struct {
	Commit, Judge  string
	Score, CostUSD float64
}

// judgeKey names the judge a holdout score came from: the sealed set,
// the pinned binary and the command line.
func (cfg Config) judgeKey() string {
	sum := sha256.Sum256([]byte(strings.Join(append([]string{cfg.HoldoutHash, cfg.BinaryHash}, cfg.JudgeArgv...), "\x00")))
	return hex.EncodeToString(sum[:])
}

func runHoldout(ctx context.Context, s Store, cfg Config, j Judge) (holdoutResult, error) {
	if j == nil {
		return holdoutResult{}, errors.New("research: stop: no judge configured")
	}
	var h holdoutResult
	r := s.repo(cfg.Name)
	dir := filepath.Join(s.Dir(cfg.Name), "holdout")
	cache := filepath.Join(dir, "result.json")
	done := map[string]heldoutSide{}
	if b, err := os.ReadFile(cache); err == nil {
		json.Unmarshal(b, &done)
	}
	key := cfg.judgeKey()
	for _, side := range []struct {
		name, commit string
		score        *float64
	}{{"original", cfg.StartCommit, &h.original}, {"winner", cfg.BestCommit, &h.winner}} {
		if c, ok := done[side.name]; ok && c.Commit == side.commit && c.Judge == key {
			*side.score = c.Score
			h.cost += c.CostUSD
			continue
		}
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
		var total, cost float64
		for i := 1; i <= 2; i++ {
			res, err := j.Run(ctx, cand, "holdout", filepath.Join(dir, fmt.Sprintf("%s-%d.json", side.name, i)))
			if err != nil {
				return h, fmt.Errorf("research: holdout on the %s: %w", side.name, err)
			}
			cost += res.CostUSD
			total += res.Score
		}
		*side.score = total / 2
		h.cost += cost
		// The cache only saves money, so a failed write is no failure.
		done[side.name] = heldoutSide{Commit: side.commit, Judge: key, Score: *side.score, CostUSD: cost}
		if b, err := json.Marshal(done); err == nil {
			os.WriteFile(cache, b, 0o600)
		}
	}
	return h, nil
}
