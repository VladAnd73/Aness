package research

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// raceBeforeNightLock runs write once, in the window just before the
// next writer of name takes its night lock.
func raceBeforeNightLock(t *testing.T, name string, write func()) {
	t.Helper()
	done := false
	prev := beforeNightLock
	beforeNightLock = func(n string) {
		if n == name && !done {
			done = true
			write()
		}
	}
	t.Cleanup(func() { beforeNightLock = prev })
}

// newBest commits body as the target in the campaign repo, the way a
// kept try does, and returns the commit.
func newBest(t *testing.T, f fixture, body string) string {
	t.Helper()
	r := f.store.repo(f.name)
	if err := os.WriteFile(filepath.Join(r.dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := r.Commit("kept by a night that finished")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A stop that finishes after the night read the config but before it
// took the lock must not be undone by that night.
func TestNightLoadsConfigAfterTakingTheLock(t *testing.T) {
	f := newCampaign(t, nil)
	raceBeforeNightLock(t, f.name, func() {
		cfg := f.load(t)
		cfg.Status, cfg.StopReason = StatusStopped, "operator"
		if err := f.store.Save(cfg); err != nil {
			t.Fatal(err)
		}
	})
	ed := &fakeEditor{}
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &fakeJudge{scores: []float64{50}}))
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Errorf("RunNight on a campaign stopped before the lock = %v, want a refusal", err)
	}
	if cfg := f.load(t); cfg.Status != StatusStopped || ed.calls != 0 {
		t.Errorf("status %s, editor calls %d; want still stopped and no try", cfg.Status, ed.calls)
	}
}

// A night that finishes between Stop's read and its lock must keep its
// counters and its new best, and the holdout must judge that best.
func TestStopLoadsConfigAfterTakingTheLock(t *testing.T) {
	f := newCampaign(t, nil)
	winner := strings.Repeat("w", 50) + "\n"
	raceBeforeNightLock(t, f.name, func() {
		cfg := f.load(t)
		cfg.BestCommit = newBest(t, f, winner)
		cfg.Nights, cfg.QuotaUsed, cfg.JudgeCostUSD = 3, 9, 1.5
		if err := f.store.Save(cfg); err != nil {
			t.Fatal(err)
		}
	})
	var judged [][]byte
	j := &fakeJudge{holdout: func(b []byte) float64 { judged = append(judged, b); return 70 }}
	if err := Stop(context.Background(), f.store, f.name, "operator", f.deps(&fakeEditor{}, j)); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cfg := f.load(t)
	if cfg.Nights != 3 || cfg.QuotaUsed != 9 || cfg.JudgeCostUSD < 1.5 {
		t.Errorf("nights %d, quota %v, judge cost %v; want the finished night's 3, 9 and at least 1.5", cfg.Nights, cfg.QuotaUsed, cfg.JudgeCostUSD)
	}
	if len(judged) != 4 || !bytes.Equal(judged[len(judged)-1], []byte(winner)) {
		t.Errorf("holdout judged %d candidates, last %q; want 4 with the new best last", len(judged), judged)
	}
}

func TestResetLoadsConfigAfterTakingTheLock(t *testing.T) {
	f := newCampaign(t, nil)
	var best string
	raceBeforeNightLock(t, f.name, func() {
		cfg := f.load(t)
		best = newBest(t, f, "kept\n")
		cfg.BestCommit = best
		if err := f.store.Save(cfg); err != nil {
			t.Fatal(err)
		}
	})
	got, err := Reset(f.store, f.name)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got != best || f.repoHead(t) != best {
		t.Errorf("reset to %s (head %s), want the best commit saved before the lock, %s", got, f.repoHead(t), best)
	}
}

func TestApproveJudgeWaitsForConfigLock(t *testing.T) {
	f := newCampaign(t, nil)
	if err := f.store.SetOn(f.name, false); err != nil {
		t.Fatal(err)
	}
	cfg := f.load(t)
	cfg.Status = StatusReady
	f.store.Save(cfg)
	bin := filepath.Join(t.TempDir(), "bin")
	os.WriteFile(bin, []byte("b"), 0o700)
	waitsFor(t, f, func() error { _, err := f.store.ApproveJudge(f.name, "testdata/drafts", bin, nil); return err })
}

// Nights share the week's quota, so one runs at a time host-wide; but a
// night of campaign A must not block settings on campaign B, while it
// still blocks them on A itself and blocks any other quota spender.
func TestNightLocksAreHostWideOnlyForSpending(t *testing.T) {
	a, b := newCampaign(t, nil), newCampaign(t, nil)
	host := b.lockPath()
	if err := b.store.SetOn(b.name, false); err != nil {
		t.Fatal(err)
	}
	var onB, setB, resetB, setA, stopB error
	during := func(path string) error {
		onB = b.store.SetOn(b.name, true)
		setB = b.store.Set(b.name, "max_nights", "9")
		_, resetB = Reset(b.store, b.name)
		setA = a.store.Set(a.name, "max_nights", "9")
		db := b.deps(&fakeEditor{}, &fakeJudge{})
		db.LockPath = host
		stopB = Stop(context.Background(), b.store, b.name, "operator", db)
		return appendBytes("x\n")(path)
	}
	d := a.deps(&fakeEditor{edits: []func(string) error{during}}, &fakeJudge{scores: []float64{50, 50, 40}})
	d.LockPath = host
	cfgA := a.load(t)
	cfgA.MaxTriesPerNight = 1
	a.store.Save(cfgA)
	if _, err := RunNight(context.Background(), a.store, a.name, d); err != nil {
		t.Fatalf("RunNight A: %v", err)
	}
	for what, err := range map[string]error{"on B": onB, "set B": setB, "reset B": resetB} {
		if err != nil {
			t.Errorf("%s during A's night: %v; want it allowed", what, err)
		}
	}
	if setA == nil {
		t.Errorf("set A during A's own night: want a refusal")
	}
	if stopB == nil {
		t.Errorf("stop B during A's night: want a refusal, its holdout spends the shared quota")
	}
	if cfg := b.load(t); cfg.Status != StatusOn || cfg.MaxNights != 9 {
		t.Errorf("B after A's night: status %s, max nights %d; want on and 9", cfg.Status, cfg.MaxNights)
	}
}

// A night or stop of a campaign that may not run must not take the
// host-wide lock even briefly: another campaign's night would be refused.
func TestNightAndStopCheckStatusBeforeHostLock(t *testing.T) {
	f := newCampaign(t, nil)
	unlock, _, err := Lock(f.lockPath(), "night of other")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := f.store.SetOn(f.name, false); err != nil {
		t.Fatal(err)
	}
	_, err = RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, &fakeJudge{}))
	if err == nil || !strings.Contains(err.Error(), "not on") {
		t.Errorf("night of an off campaign = %v; want it refused for being off, before the host lock", err)
	}
	cfg := f.load(t)
	cfg.Status = StatusStopped
	f.store.Save(cfg)
	err = Stop(context.Background(), f.store, f.name, "operator", f.deps(&fakeEditor{}, &fakeJudge{}))
	if err == nil || !strings.Contains(err.Error(), "can be stopped") {
		t.Errorf("stop of a stopped campaign = %v; want it refused for its status, before the host lock", err)
	}
}

func TestUnknownCampaignSaysNotFound(t *testing.T) {
	f := newCampaign(t, nil)
	d := f.deps(&fakeEditor{}, &fakeJudge{})
	calls := map[string]func() error{
		"reset":  func() error { _, err := Reset(f.store, "typo"); return err },
		"accept": func() error { _, err := Accept(f.store, "typo", false); return err },
		"on":     func() error { return f.store.SetOn("typo", true) },
		"off":    func() error { return f.store.SetOn("typo", false) },
		"set":    func() error { return f.store.Set("typo", "max_nights", "3") },
		"stop":   func() error { return Stop(context.Background(), f.store, "typo", "operator", d) },
		"night":  func() error { _, err := RunNight(context.Background(), f.store, "typo", d); return err },
	}
	for what, call := range calls {
		if err := call(); err == nil || !strings.Contains(err.Error(), "campaign typo") || strings.Contains(err.Error(), ".lock") {
			t.Errorf("%s typo = %v; want the campaign-not-found error, not a lock error", what, err)
		}
	}
	if _, err := os.Stat(f.store.Dir("typo")); err == nil {
		t.Errorf("a call on an unknown campaign created its folder")
	}
}

// A writer holding a campaign's lock is named for what it is doing, so
// a refused night does not blame a night that is not running.
func TestRefusalNamesTheWriterHoldingTheLock(t *testing.T) {
	f := newCampaign(t, nil)
	var nightErr, setErr error
	j := &fakeJudge{holdout: func([]byte) float64 {
		if nightErr == nil {
			_, nightErr = RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, &fakeJudge{}))
			setErr = f.store.Set(f.name, "max_nights", "3")
		}
		return 70
	}}
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	f.store.Save(cfg)
	if err := Stop(context.Background(), f.store, f.name, "operator", f.deps(&fakeEditor{}, j)); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	for what, err := range map[string]error{"night": nightErr, "set": setErr} {
		if err == nil || !strings.Contains(err.Error(), "stop of "+f.name) || strings.Contains(err.Error(), "another night") {
			t.Errorf("%s during a stop = %v; want a refusal naming the stop of %s", what, err, f.name)
		}
	}
}

// A stop whose final save fails has already spent the holdout; a retry
// must reuse its result, not run and pay for it again.
func TestStopRetryReusesTheSpentHoldout(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	f.store.Save(cfg)
	good, err := os.ReadFile(filepath.Join(f.store.Dir(f.name), "campaign.toml"))
	if err != nil {
		t.Fatal(err)
	}
	toml := filepath.Join(f.store.Dir(f.name), "campaign.toml")
	j := &fakeJudge{}
	j.holdout = func(b []byte) float64 {
		if j.holdoutCalls == 4 {
			os.WriteFile(toml, []byte("not a config\n"), 0o600)
		}
		return beatsOriginal(f)(b)
	}
	d := f.deps(&fakeEditor{}, j)
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err == nil {
		t.Fatal("Stop with an unreadable config after the holdout: want an error")
	}
	os.WriteFile(toml, good, 0o600)
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("retried Stop: %v", err)
	}
	if cfg := f.load(t); j.holdoutCalls != 4 || cfg.Status != StatusStopped || cfg.HoldoutVerdict != "pass" {
		t.Errorf("holdout calls %d, status %s, verdict %q; want 4 calls in all, stopped, pass", j.holdoutCalls, cfg.Status, cfg.HoldoutVerdict)
	}
}

// The cache only saves money; failing to write it must not fail a stop
// whose holdout was already paid for.
func TestStopSucceedsWhenHoldoutCacheCannotBeWritten(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	f.store.Save(cfg)
	if err := os.MkdirAll(filepath.Join(f.store.Dir(f.name), "holdout", "result.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	j := &fakeJudge{holdout: func([]byte) float64 { return 70 }}
	if err := Stop(context.Background(), f.store, f.name, "operator", f.deps(&fakeEditor{}, j)); err != nil {
		t.Errorf("Stop with an unwritable holdout cache: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusStopped {
		t.Errorf("status %s, want stopped", cfg.Status)
	}
}

// sideJudge fails the first holdout run of the winner and counts runs
// per side.
type sideJudge struct {
	winner        []byte
	failed        bool
	orig, winRuns int
}

func (j *sideJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	b, err := os.ReadFile(candidate)
	if err != nil {
		return JudgeResult{}, err
	}
	if !bytes.Equal(b, j.winner) {
		j.orig++
		return JudgeResult{Score: 60, Runs: 3}, nil
	}
	if !j.failed {
		j.failed = true
		return JudgeResult{}, errors.New("winner judge broke")
	}
	j.winRuns++
	return JudgeResult{Score: 70, Runs: 3}, nil
}

// A side already paid for is not paid again when the other side failed.
func TestStopRetryReusesAFinishedHoldoutSide(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	f.store.Save(cfg)
	j := &sideJudge{winner: []byte("better\n")}
	d := f.deps(&fakeEditor{}, j)
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err == nil {
		t.Fatal("Stop with a failing winner side: want an error")
	}
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("retried Stop: %v", err)
	}
	if cfg := f.load(t); j.orig != 2 || j.winRuns != 2 || cfg.HoldoutVerdict != "pass" {
		t.Errorf("original runs %d, winner runs %d, verdict %q; want 2, 2, pass", j.orig, j.winRuns, cfg.HoldoutVerdict)
	}
}

// Every quota spender refuses the same way when its locks are held.
func TestHostLockBusyIsARefusal(t *testing.T) {
	f := newCampaign(t, nil)
	unlock, _, err := Lock(f.lockPath(), "night of other")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	_, err = f.store.lockLoad(f.name, lockOp{what: "night", scope: hostNight, host: f.lockPath()})
	var refused *RefusedError
	if !errors.As(err, &refused) || !strings.Contains(refused.Reason, "night of other") {
		t.Errorf("lockLoad with the host lock held = %v; want a RefusedError naming the holder", err)
	}
}

// Raising the limit that switched the campaign off lifts the refusal.
func TestOnAfterRaisingTheLimitThatSwitchedOff(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	cfg.Status, cfg.StopReason, cfg.Nights, cfg.MaxNights = StatusOff, "max-nights", 1, 1
	f.store.Save(cfg)
	if err := f.store.SetOn(f.name, true); err == nil {
		t.Fatal("on at max-nights with the holdout pending: want a refusal")
	}
	if err := f.store.Set(f.name, "max_nights", "3"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetOn(f.name, true); err != nil {
		t.Errorf("on after raising max_nights: %v", err)
	}
}

// A manual off whose nights are used up can only end by stop; say so.
func TestOnWithNightsUsedUpPointsToStop(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit = newBest(t, f, "better\n")
	cfg.Status, cfg.StopReason, cfg.Nights, cfg.MaxNights = StatusOff, "manual", 2, 2
	f.store.Save(cfg)
	if err := f.store.SetOn(f.name, true); err == nil || !strings.Contains(err.Error(), "spore research stop "+f.name) {
		t.Errorf("on with no nights left = %v; want advice naming spore research stop %s", err, f.name)
	}
}
