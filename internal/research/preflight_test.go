package research

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightRefusesWhenLockHeld(t *testing.T) {
	f := newCampaign(t, nil)
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	d := f.deps(ed, j)
	unlock, _, err := Lock(d.LockPath, "other-campaign")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	_, err = RunNight(context.Background(), f.store, f.name, d)
	wantRefused(t, err, ed, j, "another night holds the lock", "other-campaign")
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
	unlock, _, err := Lock(d.LockPath, "next")
	if err != nil {
		t.Fatalf("lock still held after a refused night: %v", err)
	}
	unlock()
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

func TestBaselineCrashEndsNight(t *testing.T) {
	f := newCampaign(t, nil)
	ed, j := &fakeEditor{}, &fakeJudge{err: errors.New("judge broke")}
	sum, _ := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	cfg := f.load(t)
	if sum.StopReason != "baseline-crash" || cfg.Nights != 0 || cfg.BestCommit != "" || ed.calls != 0 {
		t.Errorf("stop %q, nights %d, best commit %q, editor calls %d", sum.StopReason, cfg.Nights, cfg.BestCommit, ed.calls)
	}
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
