package research

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// try runs one edit, commit, judge and keep-or-reset. It returns
// interrupted when ctx ended mid-try; the caller's cleanup then resets
// the repo and no row is written.
func (n *night) try(ctx context.Context) (interrupted bool, err error) {
	n.sum.Tries++
	k := n.sum.Tries
	tdir := filepath.Join(n.dir(), fmt.Sprintf("try-%d", k))
	scratch, err := buildScratch(n.s, n.cfg, n.num, k)
	if err != nil {
		return false, err
	}
	frozenHash, err := HashTree(filepath.Join(scratch, "frozen"))
	if err != nil {
		return false, err
	}
	rel := n.cfg.targetRel()
	res, edErr := n.d.Editor.Propose(ctx, EditRequest{ScratchDir: scratch, TargetRel: rel, Try: k})
	if res.Transcript != "" {
		os.WriteFile(filepath.Join(tdir, "editor.jsonl"), []byte(res.Transcript), 0o600)
	}
	if ctx.Err() != nil {
		return true, nil
	}
	row := Row{Try: k, Description: res.Description}
	crash := func(why string, detail error) (bool, error) {
		if detail != nil {
			os.WriteFile(filepath.Join(tdir, "crash.txt"), []byte(detail.Error()+"\n"), 0o600)
		}
		row.Status = "crash"
		row.Description = fmt.Sprintf("%s (crash: %s)", row.Description, why)
		return false, n.reset(row)
	}
	if edErr != nil || res.ExitCode != 0 {
		return crash("editor failed", edErr)
	}
	cand := filepath.Join(scratch, rel)
	fi, err := os.Lstat(cand)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
		return crash("target missing, empty or not a file", err)
	}
	if h, err := HashTree(filepath.Join(scratch, "frozen")); err != nil || h != frozenHash {
		return crash("frozen file changed", err)
	}
	body, err := os.ReadFile(cand)
	if err != nil {
		return crash("target unreadable", err)
	}
	row.Bytes = len(body)
	if err := os.WriteFile(filepath.Join(n.repo.dir, rel), body, 0o600); err != nil {
		return false, err
	}
	changed, err := n.repo.changed()
	if err != nil {
		return false, err
	}
	for _, p := range changed {
		if p != rel {
			return crash("a file other than the target changed", fmt.Errorf("changed: %v", changed))
		}
	}
	if row.Commit, err = n.repo.Commit(fmt.Sprintf("try %d.%d: %s", n.num, k, res.Description)); err != nil {
		return false, err
	}
	if OverLengthCap(n.cfg.StartBytes, row.Bytes) {
		row.Status = "rejected-length"
		return false, n.reset(row)
	}

	repoCand := filepath.Join(n.repo.dir, rel)
	start := Measured{Score: n.cfg.StartScore, Bytes: n.cfg.StartBytes}
	best := Measured{Score: n.cfg.BestScore, Bytes: n.cfg.BestBytes}
	var total float64
	for i := 1; i <= 2; i++ {
		jr, err := n.d.Judge.Run(ctx, repoCand, "main", filepath.Join(tdir, fmt.Sprintf("judge-%d.json", i)))
		if ctx.Err() != nil {
			return true, nil
		}
		if err != nil {
			os.WriteFile(filepath.Join(tdir, "crash.txt"), []byte(err.Error()+"\n"), 0o600)
			return crash("judge failed", nil)
		}
		n.cfg.JudgeCostUSD += jr.CostUSD
		total += jr.Score
		row.Score = roundScore(total / float64(i))
		if !n.obj.Better(start, best, Measured{Score: jr.Score, Bytes: row.Bytes}) {
			row.Status = "reset"
			return false, n.reset(row)
		}
	}
	row.Status = "kept"
	return false, n.keep(row)
}

// keep saves the try's commit as best before logging it, so a crash in
// between leaves a consistent config and only a missing log line.
func (n *night) keep(row Row) error {
	cfg := n.cfg
	cfg.BestCommit, cfg.BestScore, cfg.BestBytes = row.Commit, row.Score, row.Bytes
	if err := n.s.Save(cfg); err != nil {
		return err
	}
	n.cfg = cfg
	n.sum.Kept++
	row.At = n.d.Now()
	return AppendRow(n.resultsPath(), row)
}

// reset moves the repo back to the best commit, which the saved config
// already names, then logs the try.
func (n *night) reset(row Row) error {
	if err := n.repo.ResetHard(n.bestCommit()); err != nil {
		return err
	}
	if err := n.s.Save(n.cfg); err != nil {
		return err
	}
	row.At = n.d.Now()
	return AppendRow(n.resultsPath(), row)
}
