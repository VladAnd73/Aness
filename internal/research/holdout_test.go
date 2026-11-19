package research

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// keptNight runs one night whose single try is kept, scoring the
// holdout by holdout(candidate bytes).
func keptNight(t *testing.T, f fixture, holdout func([]byte) float64) (*fakeJudge, Deps) {
	t.Helper()
	ed := &fakeEditor{edits: []func(string) error{appendBytes("winner\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 80}, holdout: holdout}
	d := f.deps(ed, j)
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	return j, d
}

func TestStopNoKeptChangeSkipsHoldout(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("worse\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 40}}
	d := f.deps(ed, j)
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Fatal(err)
	}
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cfg := f.load(t)
	if j.holdoutCalls != 0 || cfg.Status != StatusStopped || cfg.HoldoutVerdict != "" || cfg.StopReason != "operator" {
		t.Errorf("holdout calls %d, status %q, verdict %q, reason %q", j.holdoutCalls, cfg.Status, cfg.HoldoutVerdict, cfg.StopReason)
	}
	if report := f.readArtifact(t, "report.md"); !strings.Contains(report, "holdout: no change") {
		t.Errorf("report.md lacks holdout: no change:\n%s", report)
	}
}

func TestStopHoldoutPass(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	j, d := keptNight(t, f, func(b []byte) float64 {
		if bytes.Equal(b, f.original) {
			return 60
		}
		return 80
	})
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if j.holdoutCalls != 4 {
		t.Errorf("holdout calls = %d, want 4 (original and winner, each with a confirmation)", j.holdoutCalls)
	}
	if cfg := f.load(t); cfg.HoldoutVerdict != "pass" || cfg.Status != StatusStopped {
		t.Errorf("verdict %q status %q", cfg.HoldoutVerdict, cfg.Status)
	}
	report := f.readArtifact(t, "report.md")
	for _, want := range []string{"start score: 50.0", "best score: 80.0", "holdout: pass (original 60.0, winner 80.0)", "length: 100 -> 107 bytes", "+winner", "kept 1", "stop reason: operator"} {
		if !strings.Contains(report, want) {
			t.Errorf("report.md lacks %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "do not accept") {
		t.Errorf("a passing holdout recommends not accepting")
	}
}

func TestStopOverfitRecommendsNotAccepting(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	_, d := keptNight(t, f, func(b []byte) float64 {
		if bytes.Equal(b, f.original) {
			return 80
		}
		return 60
	})
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if report := f.readArtifact(t, "report.md"); !strings.Contains(report, "Recommendation: do not accept (overfit on holdout).") {
		t.Errorf("report.md lacks the overfit recommendation:\n%s", report)
	}
}

func TestStopNotifiesCoordinator(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	_, d := keptNight(t, f, func([]byte) float64 { return 70 })
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(*f.notes) != 1 {
		t.Fatalf("notes = %v, want one", *f.notes)
	}
	msg := (*f.notes)[0]
	for _, want := range []string{"research demo stopped (operator)", "main 50.0 -> 80.0", "holdout pass", "report.md"} {
		if !strings.Contains(msg, want) {
			t.Errorf("notice %q lacks %q", msg, want)
		}
	}
}

func TestStopNotifyErrorIsOnlyAWarning(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	_, d := keptNight(t, f, func([]byte) float64 { return 70 })
	d.Notify = func(string) error { return errors.New("tell failed") }
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Errorf("Stop failed on a notify error: %v", err)
	}
	if !strings.Contains(f.readArtifact(t, "report.md"), "tell failed") {
		t.Errorf("the notify failure is not recorded in the report")
	}
}

func TestStopRefusesWhenLockHeld(t *testing.T) {
	f := newCampaign(t, nil)
	d := f.deps(&fakeEditor{}, &fakeJudge{})
	unlock, _, _ := Lock(d.LockPath, "other-campaign")
	defer unlock()
	err := Stop(context.Background(), f.store, f.name, "operator", d)
	if err == nil || !strings.Contains(err.Error(), "other-campaign") {
		t.Errorf("Stop = %v, want a refusal naming the lock holder", err)
	}
}

func TestStopTwiceRefuses(t *testing.T) {
	f := newCampaign(t, nil)
	d := f.deps(&fakeEditor{}, &fakeJudge{})
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatal(err)
	}
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err == nil {
		t.Errorf("second Stop succeeded")
	}
}

func TestNightSwitchingOffWithKeptChangeStops(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1; c.MaxNights = 1 })
	j, _ := keptNight(t, f, func([]byte) float64 { return 70 })
	cfg := f.load(t)
	if cfg.Status != StatusStopped || cfg.StopReason != "max-nights" || j.holdoutCalls != 4 || len(*f.notes) != 1 {
		t.Errorf("status %q reason %q holdout calls %d notes %v", cfg.Status, cfg.StopReason, j.holdoutCalls, *f.notes)
	}
}
