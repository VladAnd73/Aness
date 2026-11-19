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
