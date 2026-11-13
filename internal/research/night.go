package research

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	cfg, err := s.Load(name)
	if err != nil {
		return NightSummary{}, err
	}
	obj, err := LookupObjective(cfg.Objective)
	if err != nil {
		return NightSummary{}, err
	}
	unlock, startUsed, err := preflight(ctx, s, cfg, d)
	if err != nil {
		return NightSummary{}, err
	}
	defer unlock()
	n := &night{s: s, cfg: cfg, d: d, obj: obj, repo: s.repo(name), num: cfg.Nights + 1,
		started: d.Now(), startUsed: startUsed, lastUsed: startUsed}
	n.sum = NightSummary{Night: n.num, StartScore: cfg.BestScore}
	return n.run(ctx)
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
	if n.cfg.Nights == 0 && n.cfg.BestCommit == "" {
		if reason, err := n.baseline(ctx); reason != "" || err != nil {
			n.sum.StopReason = reason
			if err == nil {
				err = appendReport(n.s, n.cfg.Name, fmt.Sprintf("## Night %d (not counted)\n\n- stop reason: %s\n\n", n.num, reason))
			}
			return n.sum, err
		}
		n.sum.StartScore = n.cfg.BestScore
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
// start and best. A crash ends the night without counting it.
func (n *night) baseline(ctx context.Context) (stopReason string, err error) {
	cand := filepath.Join(n.repo.dir, n.cfg.targetRel())
	cfg := n.cfg
	var total float64
	for i := 1; i <= 2; i++ {
		res, err := n.d.Judge.Run(ctx, cand, "main", filepath.Join(n.dir(), fmt.Sprintf("baseline-%d.json", i)))
		if ctx.Err() != nil {
			return "interrupted", nil
		}
		if err != nil {
			os.WriteFile(filepath.Join(n.dir(), "baseline-crash.txt"), []byte(err.Error()+"\n"), 0o600)
			return "baseline-crash", nil
		}
		cfg.JudgeCostUSD += res.CostUSD
		total += res.Score
	}
	cfg.StartScore = roundScore(total / 2)
	cfg.BestScore = cfg.StartScore
	cfg.BestCommit = cfg.StartCommit
	cfg.BestBytes = cfg.StartBytes
	if err := n.s.Save(cfg); err != nil {
		return "", err
	}
	n.cfg = cfg
	return "", n.log(Row{Try: 0, Score: cfg.BestScore, Status: "baseline",
		Bytes: cfg.StartBytes, Commit: cfg.StartCommit, Description: "baseline"})
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
	if n.sum.Tries >= n.cfg.MaxTriesPerNight {
		return "max-tries"
	}
	if n.d.Now().Sub(n.started).Hours() >= n.cfg.MaxHoursPerNight {
		return "max-hours"
	}
	return ""
}

func (n *night) end(ctx context.Context) (NightSummary, error) {
	cfg := n.cfg
	cfg.Nights++
	if n.sum.Kept > 0 {
		cfg.NoGainNights = 0
	} else {
		cfg.NoGainNights++
	}
	used := max(n.lastUsed-n.startUsed, 0)
	cfg.QuotaUsed += used
	switch {
	case cfg.NoGainNights >= cfg.StopAfterNightsWithoutGain:
		cfg.Status, cfg.StopReason = StatusOff, "no-gain"
	case cfg.Nights >= cfg.MaxNights:
		cfg.Status, cfg.StopReason = StatusOff, "max-nights"
	}
	if err := n.s.Save(cfg); err != nil {
		return n.sum, err
	}
	n.cfg = cfg
	n.sum.EndScore = cfg.BestScore
	section := fmt.Sprintf("## Night %d\n\n- started: %s\n- ended: %s\n- stop reason: %s\n- tries: %d\n- kept: %d\n- score: %.1f -> %.1f\n- quota used: %.1f points\n- judge cost so far: $%.2f\n\n",
		n.num, n.started.UTC().Format(time.RFC3339), n.d.Now().UTC().Format(time.RFC3339), n.sum.StopReason,
		n.sum.Tries, n.sum.Kept, n.sum.StartScore, n.sum.EndScore, used, cfg.JudgeCostUSD)
	if err := appendReport(n.s, cfg.Name, section); err != nil {
		return n.sum, err
	}
	if cfg.Status == StatusOff && cfg.hasKeptChange() && ctx.Err() == nil {
		if err := stopCampaign(ctx, n.s, cfg, cfg.StopReason, n.d); err != nil {
			return n.sum, fmt.Errorf("research: night %d switched the campaign off, but stopping it failed: %w", n.num, err)
		}
	}
	return n.sum, nil
}
