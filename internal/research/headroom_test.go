package research

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A kept edit that reaches 100 ends the night and the campaign: no try
// can beat it, so the holdout runs instead of the night spending quota.
func TestKeptEditAtMaxScoreStopsTheCampaign(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("winner\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 100, 100}, holdout: beatsOriginal(f)}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if sum.StopReason != "max-score" || ed.calls != 1 {
		t.Errorf("stop reason %q after %d edits, want max-score after 1", sum.StopReason, ed.calls)
	}
	if cfg := f.load(t); cfg.Status != StatusStopped || cfg.HoldoutVerdict != "pass" {
		t.Errorf("status %s verdict %q, want stopped with the holdout run", cfg.Status, cfg.HoldoutVerdict)
	}
}

// shrink-holding-score improves a file that already scores 100, so a
// 100 baseline is no reason to refuse its night.
func TestShrinkObjectiveRunsOnAFullBaseline(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1; c.Objective = "shrink-holding-score" })
	ed := &fakeEditor{}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &fakeJudge{scores: []float64{100}})); err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if ed.calls != 1 {
		t.Errorf("editor calls = %d, want 1", ed.calls)
	}
}

// A no-headroom campaign takes a harder judge: approve resets it to
// ready, and the next night scores a fresh baseline.
func TestNoHeadroomCampaignTakesANewJudge(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	d := f.deps(&fakeEditor{}, &fakeJudge{scores: []float64{100}})
	var refused *RefusedError
	if _, err := RunNight(context.Background(), f.store, f.name, d); !errors.As(err, &refused) {
		t.Fatalf("night = %v, want no-headroom", err)
	}
	binary := f.load(t).JudgeArgv[0]
	if _, err := f.store.ApproveJudge(f.name, "testdata/drafts", binary, []string{binary, "eval", "judge"}); err != nil {
		t.Fatalf("ApproveJudge after no-headroom: %v", err)
	}
	cfg := f.load(t)
	if cfg.Status != StatusReady || cfg.BestCommit != "" || cfg.BestScore != 0 || cfg.StopReason != "" {
		t.Errorf("after approve: status %s best %q %.1f stop %q, want ready with no baseline", cfg.Status, cfg.BestCommit, cfg.BestScore, cfg.StopReason)
	}
	if err := f.store.SetOn(f.name, true); err != nil {
		t.Fatal(err)
	}
	j := &fakeJudge{scores: []float64{50}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, j)); err != nil {
		t.Errorf("night on the new judge: %v", err)
	}
	if got := f.load(t).StartScore; got != 50 {
		t.Errorf("start score = %.1f, want the new baseline 50", got)
	}
}

// A skill outside the spore repo is overwritten in place even inside
// another git checkout, such as a dotfiles repo; only role targets land
// on a branch there.
func TestAcceptSkillInOtherGitRepoStaysInPlace(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	root := filepath.Dir(f.liveDir)
	gitIn(t, root, "init", "--quiet")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "-c", "user.name=t", "-c", "user.email=t@localhost", "commit", "--quiet", "-m", "dotfiles")
	stoppedWinner(t, f)
	res, err := Accept(f.store, f.name, false)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.Branch != "" || res.Backup == "" {
		t.Errorf("accept result %+v, want an in-place write with a backup", res)
	}
}

// --step is matched to the target's role before it counts as ambiguous.
func TestRoleStepPickedByRoleAcrossLoops(t *testing.T) {
	root, target := newRoleProject(t)
	other := strings.ReplaceAll(roleLoopTOML, `name = "qa"`, `name = "other"`)
	other = strings.ReplaceAll(other, `role = "qa-analyst"`, `role = "qa-reviewer"`)
	other = strings.ReplaceAll(other, `name = "review"`, `name = "check"`)
	if err := os.WriteFile(filepath.Join(root, "bootstrap", "loops", "other.toml"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	setup, _, err := resolveRole(target, RoleOptions{Step: "intake"})
	if err != nil {
		t.Fatalf("resolveRole: %v", err)
	}
	if setup.step.Loop != "qa" {
		t.Errorf("loop = %q, want qa, the one whose intake runs qa-analyst", setup.step.Loop)
	}
}

// The judge scores context/step.json; a campaign.toml whose step says
// otherwise is refused rather than reported as a different step.
func TestNightRefusesStepDrift(t *testing.T) {
	f, _ := newRoleCampaign(t, nil)
	cfg := f.load(t)
	cfg.StepOutput = "elsewhere/round-{round}.json"
	f.store.Save(cfg)
	_, err := RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, &fakeJudge{}))
	var refused *RefusedError
	if !errors.As(err, &refused) || !strings.Contains(refused.Reason, "step.json") {
		t.Errorf("night = %v, want a refusal naming step.json", err)
	}
}

// The retry keys on the campaign's state, not its stop reason: an on and
// an off after the no-headroom night must not lock it out. It also
// starts the night and no-gain counts over.
func TestNoHeadroomRetrySurvivesOnOff(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxNights = 2 })
	d := f.deps(&fakeEditor{}, &fakeJudge{scores: []float64{100}})
	RunNight(context.Background(), f.store, f.name, d)
	if err := f.store.SetOn(f.name, true); err != nil {
		t.Fatalf("on: %v", err)
	}
	if err := f.store.SetOn(f.name, false); err != nil {
		t.Fatalf("off: %v", err)
	}
	binary := f.load(t).JudgeArgv[0]
	if _, err := f.store.ApproveJudge(f.name, "testdata/drafts", binary, []string{binary, "eval", "judge"}); err != nil {
		t.Fatalf("ApproveJudge after on and off: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusReady || cfg.Nights != 0 || cfg.NoGainNights != 0 {
		t.Errorf("status %s nights %d no-gain %d, want ready with counts reset", cfg.Status, cfg.Nights, cfg.NoGainNights)
	}
}

// A campaign switched off at max-score whose holdout never ran can only
// end by stop; on would spend a night with no tries.
func TestOnRefusesMaxScoreAwaitingHoldout(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	cfg.BestCommit, cfg.BestScore, cfg.Status, cfg.StopReason = newBest(t, f, "winner\n"), 100, StatusOff, "max-score"
	f.store.Save(cfg)
	if err := f.store.SetOn(f.name, true); err == nil || !strings.Contains(err.Error(), "research stop") {
		t.Errorf("on = %v, want a refusal naming spore research stop", err)
	}
}

// A try that ties the best score but is longer is reset, not kept.
func TestEqualScoreLongerTryIsReset(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("better\n"), appendBytes("longer\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 80, 80}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	rows := f.resultRows(t)
	if len(rows) != 3 || rows[1]["status"] != "kept" || rows[2]["status"] != "reset" {
		t.Errorf("rows %v, want baseline, kept, reset", rows)
	}
	if cfg := f.load(t); cfg.BestBytes != 107 {
		t.Errorf("best bytes %d, want the shorter kept try's 107", cfg.BestBytes)
	}
}

// shrink-holding-score wants the same score from a shorter file, so a
// holdout tie, even at 100, passes; beat-not-tie is maximize-score's.
func TestShrinkObjectiveHoldoutTiePasses(t *testing.T) {
	for _, score := range []float64{80, 100} {
		f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1; c.Objective = "shrink-holding-score" })
		ed := &fakeEditor{edits: []func(string) error{writeBytes(60)}}
		j := &fakeJudge{scores: []float64{50, 50, 50, 50}, holdout: func([]byte) float64 { return score }}
		if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
			t.Fatalf("RunNight: %v", err)
		}
		if err := Stop(context.Background(), f.store, f.name, "operator", f.deps(ed, j)); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if got := f.load(t).HoldoutVerdict; got != "pass" {
			t.Errorf("shrink holdout tie at %v: verdict %q, want pass", score, got)
		}
	}
}
