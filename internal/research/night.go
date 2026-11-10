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
	cost      float64
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
	if err := os.MkdirAll(n.dir(), 0o700); err != nil {
		return n.sum, err
	}
	if n.cfg.Nights == 0 && n.cfg.BestCommit == "" {
		if reason, err := n.baseline(ctx); reason != "" || err != nil {
			n.sum.StopReason = reason
			return n.sum, err
		}
		n.sum.StartScore = n.cfg.BestScore
	}
	for {
		if reason := n.capReached(ctx); reason != "" {
			n.sum.StopReason = reason
			break
		}
		n.sum.Tries++
	}
	return n.end()
}

// baseline judges the start file twice and records the mean as both
// start and best. A crash ends the night without counting it.
func (n *night) baseline(ctx context.Context) (stopReason string, err error) {
	cand := filepath.Join(n.repo.dir, n.cfg.targetRel())
	var total float64
	for i := 1; i <= 2; i++ {
		res, err := n.d.Judge.Run(ctx, cand, "main", filepath.Join(n.dir(), fmt.Sprintf("baseline-%d.json", i)))
		if ctx.Err() != nil {
			return "interrupted", nil
		}
		if err != nil {
			return "baseline-crash", nil
		}
		n.cost += res.CostUSD
		total += res.Score
	}
	n.cfg.StartScore = roundScore(total / 2)
	n.cfg.BestScore = n.cfg.StartScore
	n.cfg.BestCommit = n.cfg.StartCommit
	n.cfg.BestBytes = n.cfg.StartBytes
	n.cfg.JudgeCostUSD += n.cost
	if err := n.s.Save(n.cfg); err != nil {
		return "", err
	}
	return "", AppendRow(n.resultsPath(), Row{Try: 0, At: n.d.Now(), Score: n.cfg.BestScore, Status: "baseline",
		Bytes: n.cfg.StartBytes, Commit: n.cfg.StartCommit, Description: "baseline"})
}

func (n *night) capReached(ctx context.Context) string {
	if ctx.Err() != nil {
		return "interrupted"
	}
	if n.sum.Tries >= n.cfg.MaxTriesPerNight {
		return "max-tries"
	}
	return ""
}

func (n *night) end() (NightSummary, error) {
	n.cfg.Nights++
	n.sum.EndScore = n.cfg.BestScore
	return n.sum, n.s.Save(n.cfg)
}
