package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type Deps struct {
	Editor     Editor
	Judge      Judge
	Room       func() (code int, line string)
	WeeklyUsed func(ctx context.Context) (float64, error)
	Notify     func(msg string) error
	Now        func() time.Time
	LockPath   string
	// Progress, when set, sees every results.tsv row right after it is written.
	Progress func(Row)
}

func (d Deps) withDefaults() Deps {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.LockPath == "" {
		d.LockPath = DefaultLockPath()
	}
	return d
}

// RefusedError is a night that did not start; Reason is one line.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return "research: night refused: " + e.Reason }

type NightSummary struct {
	Night, Tries, Kept   int
	StopReason           string
	StartScore, EndScore float64
}

// night is one RunNight in progress. cfg is the single source of truth
// for best commit, best score and counters; the repo is moved to match
// it, never the other way round.
type night struct {
	s         Store
	cfg       Config
	d         Deps
	obj       Objective
	repo      repo
	num       int
	started   time.Time
	startUsed float64
	lastUsed  float64
	sum       NightSummary
	// judgeFailures counts tries in a row whose judge failed outright.
	judgeFailures int
}

func (n *night) dir() string { return filepath.Join(n.s.Dir(n.cfg.Name), "nights", fmt.Sprint(n.num)) }

func (n *night) resultsPath() string { return filepath.Join(n.s.Dir(n.cfg.Name), "results.tsv") }

func (n *night) bestCommit() string {
	if n.cfg.BestCommit == "" {
		return n.cfg.StartCommit
	}
	return n.cfg.BestCommit
}

func RunNight(ctx context.Context, s Store, name string, d Deps) (NightSummary, error) {
	d = d.withDefaults()
	h, startUsed, err := preflight(ctx, s, name, d)
	if err != nil {
		return NightSummary{}, err
	}
	defer h.release()
	cfg := h.cfg
	obj, err := LookupObjective(cfg.Objective)
	if err != nil {
		return NightSummary{}, err
	}
	n := &night{s: s, cfg: cfg, d: d, obj: obj, repo: s.repo(name), num: cfg.Nights + 1,
		started: d.Now(), startUsed: startUsed, lastUsed: startUsed}
	n.sum = NightSummary{Night: n.num, StartScore: cfg.BestScore}
	sum, err := n.run(ctx)
	if err == nil && sum.StopReason == "no-headroom" {
		err = noHeadroom(n.cfg)
	}
	return sum, err
}

// noHeadroom refuses a night on a baseline that already scores 100 on
// the main set: no edit can show a gain, so a night would only spend
// quota.
func noHeadroom(cfg Config) error {
	return &RefusedError{Reason: fmt.Sprintf("no-headroom: campaign %s scores %.1f on the main set, so no edit can show a gain; draft harder scenarios and run `spore research judge approve %s` again", cfg.Name, cfg.BestScore, cfg.Name)}
}

// atMaxScore is a best score no try can beat. Only maximize-score is
// capped by it: shrink-holding-score improves a file at 100 by length.
func (cfg Config) atMaxScore() bool {
	return cfg.Objective == "maximize-score" && cfg.BestCommit != "" && cfg.BestScore >= 100
}

func (n *night) run(ctx context.Context) (NightSummary, error) {
	// A crash after a try's commit but before its config save leaves
	// HEAD ahead of the best commit; the config wins.
	if head, err := n.repo.Head(); err != nil {
		return n.sum, err
	} else if head != n.bestCommit() {
		if err := n.repo.ResetHard(n.bestCommit()); err != nil {
			return n.sum, err
		}
	}
	// Whatever ends the night, the repo is left on the best commit.
	defer func() { n.repo.ResetHard(n.bestCommit()) }()
	if err := os.MkdirAll(n.dir(), 0o700); err != nil {
		return n.sum, err
	}
	if n.cfg.BestCommit == "" {
		reason, err := n.baseline(ctx)
		if err != nil {
			return n.sum, err
		}
		n.sum.StopReason, n.sum.StartScore = reason, n.cfg.BestScore
		if reason == "" && n.cfg.atMaxScore() {
			n.sum.StopReason = "no-headroom"
		}
	}
	for n.sum.StopReason == "" {
		if n.sum.StopReason = n.capReached(ctx, n.sum.Tries > 0); n.sum.StopReason != "" {
			break
		}
		interrupted, err := n.try(ctx)
		if err != nil {
			return n.sum, err
		}
		if interrupted {
			n.sum.StopReason = "interrupted"
		}
	}
	return n.end(ctx)
}

// baseline judges the start file twice and records the mean as both
// start and best. A crash ends the night, which still counts: it spent
// quota. The next night retries the baseline.
func (n *night) baseline(ctx context.Context) (stopReason string, err error) {
	cand := filepath.Join(n.repo.dir, n.cfg.targetRel())
	var total, cost float64
	for i := 1; i <= 2; i++ {
		res, err := n.d.Judge.Run(ctx, cand, "main", filepath.Join(n.dir(), fmt.Sprintf("baseline-%d.json", i)))
		if ctx.Err() != nil {
			return "interrupted", nil
		}
		if err != nil {
			// The same two-strike rule as a try's judge.
			if errors.Is(err, ErrJudgeFailed) {
				if n.judgeFailures++; n.judgeFailures < 2 {
					i--
					continue
				}
			}
			os.WriteFile(filepath.Join(n.dir(), "baseline-crash.txt"), []byte(err.Error()+"\n"), 0o600)
			return "baseline-crash", nil
		}
		n.judgeFailures = 0
		cost += res.CostUSD
		total += res.Score
	}
	if err := n.update(func(cfg *Config) {
		cfg.JudgeCostUSD += cost
		cfg.StartScore = roundScore(total / 2)
		cfg.BestScore = cfg.StartScore
		cfg.BestCommit = cfg.StartCommit
		cfg.BestBytes = cfg.StartBytes
	}); err != nil {
		return "", err
	}
	return "", n.log(Row{Try: 0, Score: n.cfg.BestScore, Status: "baseline",
		Bytes: n.cfg.StartBytes, Commit: n.cfg.StartCommit, Description: "baseline"})
}

// capReached is checked before every try. afterTry adds the live quota
// read; before the first try the preflight reading stands.
func (n *night) capReached(ctx context.Context, afterTry bool) string {
	if ctx.Err() != nil {
		return "interrupted"
	}
	if afterTry {
		used, err := n.d.WeeklyUsed(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "interrupted"
			}
			return "quota-read-failed"
		}
		n.lastUsed = used
		if used-n.startUsed >= n.cfg.QuotaCeilingPoints {
			return "quota-ceiling"
		}
	}
	if n.cfg.Status != StatusOn {
		return "switched-off"
	}
	if n.cfg.atMaxScore() {
		return "max-score"
	}
	if n.sum.Tries >= n.cfg.MaxTriesPerNight {
		return "max-tries"
	}
	if n.d.Now().Sub(n.started).Hours() >= n.cfg.MaxHoursPerNight {
		return "max-hours"
	}
	return ""
}

// cutShort stop reasons end a night before it could show whether the
// target can still improve.
var cutShort = []string{"interrupted", "quota-read-failed", "switched-off", "baseline-crash", "judge-failed", "no-headroom"}

func (n *night) end(ctx context.Context) (NightSummary, error) {
	used := max(n.lastUsed-n.startUsed, 0)
	autoOff := false
	err := n.update(func(cfg *Config) {
		// Every night spent quota, so every night counts toward
		// max_nights. One cut short, or paused by an off, with nothing
		// kept is no evidence of no gain.
		cfg.Nights++
		if n.sum.StopReason == "no-headroom" {
			cfg.Status, cfg.StopReason = StatusOff, "no-headroom"
		}
		if n.sum.Kept > 0 {
			cfg.NoGainNights = 0
		} else if !slices.Contains(cutShort, n.sum.StopReason) && cfg.Status != StatusOff {
			cfg.NoGainNights++
		}
		// A limit ends the campaign even after a manual off, which would
		// otherwise leave it unable to go back on with no holdout run.
		autoOff = true
		switch {
		case cfg.NoGainNights >= cfg.StopAfterNightsWithoutGain:
			cfg.Status, cfg.StopReason = StatusOff, "no-gain"
		case cfg.Nights >= cfg.MaxNights:
			cfg.Status, cfg.StopReason = StatusOff, "max-nights"
		case n.sum.StopReason == "max-score":
			cfg.Status, cfg.StopReason = StatusOff, "max-score"
		default:
			autoOff = false
		}
		cfg.QuotaUsed += used
	})
	if err != nil {
		return n.sum, err
	}
	cfg := n.cfg
	n.sum.EndScore = cfg.BestScore
	section := fmt.Sprintf("## Night %d\n\n- started: %s\n- ended: %s\n- stop reason: %s\n- tries: %d\n- kept: %d\n- score: %.1f -> %.1f\n- quota used: %.1f points\n- judge cost so far: $%.2f\n\n",
		n.num, n.started.UTC().Format(time.RFC3339), n.d.Now().UTC().Format(time.RFC3339), n.sum.StopReason,
		n.sum.Tries, n.sum.Kept, n.sum.StartScore, n.sum.EndScore, used, cfg.JudgeCostUSD)
	if err := appendReport(n.s, cfg.Name, section); err != nil {
		return n.sum, err
	}
	// A manual off is a pause; only the engine's own switch-off ends the
	// campaign.
	if !autoOff || !cfg.hasKeptChange() {
		return n.sum, nil
	}
	if ctx.Err() != nil {
		return n.sum, n.tellHoldoutPending(cfg)
	}
	if err := stopCampaign(ctx, n.s, cfg, cfg.StopReason, n.d); err != nil {
		return n.sum, fmt.Errorf("research: night %d switched the campaign off, but stopping it failed: %w", n.num, err)
	}
	return n.sum, nil
}

// tellHoldoutPending is an interrupted night that reached a limit: the
// holdout cannot run on a cancelled context, so the operator is told how
// to finish the campaign.
func (n *night) tellHoldoutPending(cfg Config) error {
	msg := fmt.Sprintf("research %s switched off (%s) but the night was interrupted before the holdout; run `spore research stop %s` to run it and end the campaign",
		cfg.Name, cfg.StopReason, cfg.Name)
	if err := appendReport(n.s, cfg.Name, msg+"\n\n"); err != nil {
		return err
	}
	if n.d.Notify == nil {
		return appendReport(n.s, cfg.Name, "Warning: no coordinator notice configured.\n\n")
	}
	if err := n.d.Notify(msg); err != nil {
		return appendReport(n.s, cfg.Name, fmt.Sprintf("Warning: the coordinator notice failed: %v\n\n", err))
	}
	return nil
}
