package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightRefusesWhenLockHeld(t *testing.T) {
	f := newCampaign(t, nil)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	d := f.deps(ed, j)
	unlock, _, err := Lock(d.LockPath, "night of other-campaign")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	_, err = RunNight(context.Background(), f.store, f.name, d)
	wantRefused(t, err, ed, j, "the night of other-campaign holds the lock")
}

func TestPreflightRefusesDirtyRepo(t *testing.T) {
	f := newCampaign(t, nil)
	os.WriteFile(filepath.Join(f.repoDir(), "stray.md"), []byte("x"), 0o600)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	wantRefused(t, err, ed, j, "uncommitted changes")
}

func TestPreflightRefusesBinaryHashMismatch(t *testing.T) {
	f := newCampaign(t, nil)
	os.WriteFile(filepath.Join(f.store.Dir(f.name), "bin", "spore"), []byte("swapped"), 0o700)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	wantRefused(t, err, ed, j, "judge hash mismatch: binary")
}

func TestPreflightRefusesEmptyGoal(t *testing.T) {
	f := newCampaign(t, nil)
	os.WriteFile(filepath.Join(f.store.Dir(f.name), "goal.md"), []byte(" \n"), 0o600)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	wantRefused(t, err, ed, j, "goal")
}

func TestPreflightRefusesWhenStartQuotaReadFails(t *testing.T) {
	f := newCampaign(t, nil)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	d := f.deps(ed, j)
	d.WeeklyUsed = func(context.Context) (float64, error) { return 0, errors.New("network down") }
	_, err := RunNight(context.Background(), f.store, f.name, d)
	wantRefused(t, err, ed, j, "quota-read-failed")
}

func TestPreflightReleasesLockOnRefusal(t *testing.T) {
	f := newCampaign(t, nil)
	d := f.deps(&fakeEditor{}, &fakeJudge{})
	d.Room = func() (int, string) { return 1, "no room" }
	RunNight(context.Background(), f.store, f.name, d)
	for _, p := range []string{d.LockPath, f.store.nightLockPath(f.name)} {
		unlock, _, err := Lock(p, "next")
		if err != nil {
			t.Fatalf("lock %s still held after a refused night: %v", p, err)
		}
		unlock()
	}
}

func TestBaselineRunsJudgeTwiceAndRecordsMean(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 0 })
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50, 70}}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if j.calls != 2 || cfg.StartScore != 60 || cfg.BestScore != 60 || cfg.BestCommit != cfg.StartCommit {
		t.Errorf("judge calls %d, start %v, best %v, best commit %q vs start %q", j.calls, cfg.StartScore, cfg.BestScore, cfg.BestCommit, cfg.StartCommit)
	}
	if row := f.lastRow(t); row["status"] != "baseline" || row["score"] != "60.0" || row["try"] != "0" {
		t.Errorf("baseline row = %v", row)
	}
	if ed.calls != 0 || sum.StopReason != "max-tries" || cfg.Nights != 1 {
		t.Errorf("editor calls %d, stop %q, nights %d", ed.calls, sum.StopReason, cfg.Nights)
	}
}

// A judge that cannot score the start file still spent quota: the
// night counts toward max_nights and keeps its own folder, and the next
// night retries the baseline.
func TestBaselineCrashCountsTheNight(t *testing.T) {
	f := newCampaign(t, nil)
	for night := 1; night <= 2; night++ {
		ed, j := &fakeEditor{}, &fakeJudge{err: errors.New("judge broke")}
		sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
		if err != nil {
			t.Fatalf("night %d: RunNight: %v", night, err)
		}
		cfg := f.load(t)
		if sum.StopReason != "baseline-crash" || cfg.Nights != night || cfg.NoGainNights != 0 || cfg.BestCommit != "" || ed.calls != 0 {
			t.Errorf("night %d: stop %q, nights %d, no-gain %d, best commit %q, editor calls %d", night, sum.StopReason, cfg.Nights, cfg.NoGainNights, cfg.BestCommit, ed.calls)
		}
		if _, err := os.Stat(filepath.Join(f.store.Dir(f.name), "nights", fmt.Sprint(night), "baseline-crash.txt")); err != nil {
			t.Errorf("night %d has no crash note of its own: %v", night, err)
		}
	}
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{60}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Fatalf("night 3: %v", err)
	}
	if cfg := f.load(t); cfg.BestCommit != cfg.StartCommit || cfg.StartScore != 60 || cfg.Nights != 3 {
		t.Errorf("night 3: best %q start score %v nights %d; want the baseline recorded", cfg.BestCommit, cfg.StartScore, cfg.Nights)
	}
}

func TestInterruptedBaselineCountsTheNight(t *testing.T) {
	f := newCampaign(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	j := &cancelJudge{cancel: cancel}
	sum, err := RunNight(ctx, f.store, f.name, f.deps(&fakeEditor{}, j))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); sum.StopReason != "interrupted" || cfg.Nights != 1 || cfg.NoGainNights != 0 || cfg.BestCommit != "" {
		t.Errorf("stop %q, nights %d, no-gain %d, best %q", sum.StopReason, cfg.Nights, cfg.NoGainNights, cfg.BestCommit)
	}
}

type cancelJudge struct{ cancel func() }

func (j *cancelJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	j.cancel()
	return JudgeResult{}, ctx.Err()
}

func TestNightResetsHeadAheadOfBestCommit(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 0 })
	r := f.store.repo(f.name)
	os.WriteFile(filepath.Join(f.repoDir(), "SKILL.md"), []byte("half-kept try\n"), 0o600)
	if _, err := r.Commit("try 1.1: crashed before the config save"); err != nil {
		t.Fatal(err)
	}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, &fakeJudge{scores: []float64{50}})); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	f.wantBaselineHead(t, f.load(t))
}
