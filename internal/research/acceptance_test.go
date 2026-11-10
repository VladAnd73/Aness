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

// wantRefused checks err is a *RefusedError whose reason names every
// word in want, and that no agent was called.
func wantRefused(t *testing.T, err error, ed *fakeEditor, j *fakeJudge, want ...string) {
	t.Helper()
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Errorf("expected refusal (*RefusedError), got %v", err)
	} else {
		for _, w := range want {
			if !strings.Contains(refused.Reason, w) {
				t.Errorf("refusal reason %q does not name %q", refused.Reason, w)
			}
		}
	}
	if ed.calls != 0 || j.calls != 0 || j.holdoutCalls != 0 {
		t.Errorf("refused night still called agents: editor %d, judge main %d, judge holdout %d", ed.calls, j.calls, j.holdoutCalls)
	}
}

func TestAcceptance01NightRefusesWhenOff(t *testing.T) {
	f := newCampaign(t, nil)
	if err := f.store.SetOn(f.name, false); err != nil {
		t.Fatalf("SetOn(false): %v", err)
	}
	ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	wantRefused(t, err, ed, j, "off")
}

func TestAcceptance02NightRefusesWithoutBudgetRoom(t *testing.T) {
	for _, code := range []int{1, 2} {
		t.Run(map[int]string{1: "no-room", 2: "unknown"}[code], func(t *testing.T) {
			f := newCampaign(t, nil)
			ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
			d := f.deps(ed, j)
			d.Room = func() (int, string) { return code, "no room" }
			_, err := RunNight(context.Background(), f.store, f.name, d)
			wantRefused(t, err, ed, j, "budget room")
		})
	}
}

func TestAcceptance03NightRefusesOnJudgeHashMismatch(t *testing.T) {
	for _, set := range []struct{ name, scenario string }{{"main", "m1"}, {"holdout", "h1"}} {
		t.Run(set.name, func(t *testing.T) {
			f := newCampaign(t, nil)
			dir := filepath.Join(f.store.Dir(f.name), "judge", set.name, set.scenario)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			fh, err := os.OpenFile(filepath.Join(dir, "scenario.json"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fh.WriteString(" "); err != nil {
				t.Fatal(err)
			}
			fh.Close()
			ed, j := &fakeEditor{}, &fakeJudge{scores: []float64{50}}
			_, err = RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
			wantRefused(t, err, ed, j, "hash mismatch", set.name)
		})
	}
}

func TestAcceptance04WorseEditIsReset(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("worse\n")}}
	j := &fakeJudge{scores: []float64{60, 60, 40}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Errorf("RunNight: %v", err)
	}
	cfg := f.load(t)
	f.wantBaselineHead(t, cfg)
	got, err := os.ReadFile(filepath.Join(f.repoDir(), "SKILL.md"))
	if err != nil {
		t.Errorf("read campaign target: %v", err)
	} else if !bytes.Equal(got, f.original) {
		t.Errorf("campaign target was not reset: got %d bytes, want the %d start bytes", len(got), len(f.original))
	}
	row := f.lastRow(t)
	if row["status"] != "reset" || row["score"] != "40.0" {
		t.Errorf("last results.tsv row = %v, want status reset and score 40.0", row)
	}
}

func TestAcceptance05LuckyResultRejected(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("lucky\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 40}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if j.calls != 4 {
		t.Errorf("judge calls = %d, want 4 (baseline pair, try, confirmation)", j.calls)
	}
	if row := f.lastRow(t); row["status"] != "reset" {
		t.Errorf("last results.tsv row = %v, want status reset", row)
	}
	cfg := f.load(t)
	if cfg.BestScore != 50 {
		t.Errorf("cfg.BestScore = %v, want 50", cfg.BestScore)
	}
	f.wantBaselineHead(t, cfg)
}

func TestAcceptance06aMaximizeScoreTieShorterWins(t *testing.T) {
	obj, err := LookupObjective("maximize-score")
	if err != nil || obj == nil {
		t.Errorf("LookupObjective(maximize-score) = %v, %v; want an objective", obj, err)
		return
	}
	start := Measured{Score: 50, Bytes: 100}
	if !obj.Better(start, Measured{Score: 80, Bytes: 100}, Measured{Score: 80, Bytes: 90}) {
		t.Errorf("equal score, shorter candidate: want better")
	}
	if obj.Better(start, Measured{Score: 80, Bytes: 90}, Measured{Score: 80, Bytes: 100}) {
		t.Errorf("equal score, longer candidate: want not better")
	}
}

func TestAcceptance06bLengthCapRejectsWithoutJudging(t *testing.T) {
	if !OverLengthCap(100, 171) || OverLengthCap(100, 170) {
		t.Errorf("OverLengthCap(100, 171) = %v, OverLengthCap(100, 170) = %v; want true, false", OverLengthCap(100, 171), OverLengthCap(100, 170))
	}
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{writeBytes(171)}}
	j := &fakeJudge{scores: []float64{50, 50}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if j.calls != 2 {
		t.Errorf("judge calls = %d, want 2 (baseline pair only, the try is never judged)", j.calls)
	}
	if row := f.lastRow(t); row["status"] != "rejected-length" || row["bytes"] != "171" {
		t.Errorf("last results.tsv row = %v, want status rejected-length and bytes 171", row)
	}
	f.wantBaselineHead(t, f.load(t))
}

func TestAcceptance09HoldoutOverfitBlocksAccept(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("winner\n")}}
	j := &fakeJudge{
		scores: []float64{50, 50, 80, 80},
		holdout: func(b []byte) float64 {
			if bytes.Equal(b, f.original) {
				return 80
			}
			return 60
		},
	}
	d := f.deps(ed, j)
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if j.holdoutCalls == 0 {
		t.Errorf("Stop never ran the holdout set")
	}
	if report := f.readArtifact(t, "report.md"); !strings.Contains(report, "overfit") {
		t.Errorf("report.md does not say overfit:\n%s", report)
	}

	_, err := Accept(f.store, f.name, false)
	if err == nil || !strings.Contains(err.Error(), "overfit") {
		t.Errorf("Accept(force=false) = %v, want a refusal naming overfit", err)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, f.original) {
		t.Errorf("refused accept changed the live target")
	}

	res, err := Accept(f.store, f.name, true)
	if err != nil {
		t.Errorf("Accept(force=true): %v", err)
	}
	if res.Written != f.liveTarget {
		t.Errorf("Accept wrote %q, want the live target %q", res.Written, f.liveTarget)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, append(append([]byte{}, f.original...), "winner\n"...)) {
		t.Errorf("live target after forced accept = %q, want the winner", live)
	}
	backup, err := os.ReadFile(res.Backup)
	if err != nil {
		t.Errorf("read backup %q: %v", res.Backup, err)
	} else if !bytes.Equal(backup, f.original) {
		t.Errorf("backup = %q, want the original bytes", backup)
	}
}

func TestAcceptance11QuotaCeilingEndsNight(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n")}}
	j := &fakeJudge{scores: []float64{50}}
	d := f.deps(ed, j)
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) {
		reads++
		if reads == 1 {
			return 20.0, nil
		}
		return 36.0, nil
	}
	sum, err := RunNight(context.Background(), f.store, f.name, d)
	if err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if ed.calls != 1 {
		t.Errorf("editor calls = %d, want 1 (the ceiling is read after the first try)", ed.calls)
	}
	if sum.StopReason != "quota-ceiling" {
		t.Errorf("StopReason = %q, want quota-ceiling", sum.StopReason)
	}
	if report := f.readArtifact(t, "report.md"); !strings.Contains(report, "quota-ceiling") {
		t.Errorf("report.md does not name quota-ceiling:\n%s", report)
	}
}

func TestAcceptance12NoGainThreeNightsSwitchesOff(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n"), appendBytes("three\n")}}
	j := &fakeJudge{scores: []float64{50}}
	for night := 1; night <= 3; night++ {
		if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
			t.Errorf("night %d: RunNight: %v", night, err)
		}
		cfg := f.load(t)
		if night < 3 {
			if cfg.Status != StatusOn || cfg.NoGainNights != night {
				t.Errorf("after night %d: status %q, no-gain nights %d; want on, %d", night, cfg.Status, cfg.NoGainNights, night)
			}
			continue
		}
		if cfg.Status != StatusOff || cfg.StopReason != "no-gain" {
			t.Errorf("after night 3: status %q, stop reason %q; want off, no-gain", cfg.Status, cfg.StopReason)
		}
	}
}
