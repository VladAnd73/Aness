package research

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateCopiesFolderAndCommits(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	if b, err := os.ReadFile(filepath.Join(f.repoDir(), "reference.md")); err != nil || string(b) != "frozen\n" {
		t.Errorf("frozen sibling in repo = %q, %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.repoDir(), "SKILL.md")); !bytes.Equal(b, f.original) {
		t.Errorf("repo SKILL.md = %q, want the original", b)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, f.original) {
		t.Errorf("live target changed")
	}
	if fi, err := os.Stat(f.store.Dir(f.name)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("campaign dir stat = %v, %v; want mode 0700", fi, err)
	}
	if cfg.StartCommit == "" || f.repoHead(t) != cfg.StartCommit {
		t.Errorf("StartCommit %q, HEAD %q", cfg.StartCommit, f.repoHead(t))
	}
	if cfg.Target != f.liveTarget || cfg.SkillName != fixtureName || cfg.StartBytes != 100 {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.MaxTriesPerNight != 20 || cfg.MaxHoursPerNight != 8 || cfg.QuotaCeilingPoints != 15 || cfg.MaxNights != 7 || cfg.StopAfterNightsWithoutGain != 3 || cfg.Repeats != 3 || cfg.Parallel != 3 {
		t.Errorf("defaults not the spec values: %+v", cfg)
	}
	if h, _ := HashTree(f.liveDir); cfg.LiveFolderHash != h {
		t.Errorf("LiveFolderHash %q, want %q", cfg.LiveFolderHash, h)
	}
	if g, _ := os.ReadFile(filepath.Join(f.store.Dir(f.name), "goal.md")); !strings.Contains(string(g), "pass more") {
		t.Errorf("goal.md = %q", g)
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	f := newCampaign(t, nil)
	goal := filepath.Join(t.TempDir(), "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	if _, err := f.store.Create(f.name, f.liveTarget, goal, "maximize-score"); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("second Create = %v, want an exists refusal", err)
	}
}

func TestCreateRefusesNonRegularTarget(t *testing.T) {
	s := Store{Root: t.TempDir()}
	goal := filepath.Join(t.TempDir(), "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	if _, err := s.Create("x", t.TempDir(), goal, "maximize-score"); err == nil {
		t.Errorf("Create on a directory target succeeded")
	}
}

func TestApproveJudgePinsBinary(t *testing.T) {
	f := newCampaign(t, nil)
	cfg := f.load(t)
	pinned := filepath.Join(f.store.Dir(f.name), "bin", "spore")
	h, err := HashFile(pinned)
	if err != nil || h != cfg.BinaryHash {
		t.Errorf("pinned binary hash %q (%v), cfg.BinaryHash %q", h, err, cfg.BinaryHash)
	}
	if fi, _ := os.Stat(pinned); fi == nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("pinned binary mode wrong: %v", fi)
	}
	if mh, _ := HashTree(filepath.Join(f.store.Dir(f.name), "judge", "main")); mh != cfg.MainHash || mh == "" {
		t.Errorf("MainHash %q, tree %q", cfg.MainHash, mh)
	}
}

func TestApproveJudgeDefaultArgvUsesPinnedCopy(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: filepath.Join(root, "store")}
	live := filepath.Join(root, "demo")
	os.MkdirAll(live, 0o700)
	os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("x\n"), 0o600)
	goal := filepath.Join(root, "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	bin := filepath.Join(root, "bin")
	os.WriteFile(bin, []byte("b"), 0o700)
	if _, err := s.Create("c", filepath.Join(live, "SKILL.md"), goal, "maximize-score"); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.ApproveJudge("c", "testdata/drafts", bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(cfg.JudgeArgv, " ")
	dir := s.Dir("c")
	for _, want := range []string{filepath.Join(dir, "bin", "spore") + " eval judge", "--scenarios-root " + filepath.Join(dir, "judge"), "--frozen " + filepath.Join(dir, "repo"), "--skill demo", "--repeats 3", "--isolation bwrap"} {
		if !strings.Contains(argv, want) {
			t.Errorf("JudgeArgv %q lacks %q", argv, want)
		}
	}
	if cfg.Status != StatusReady {
		t.Errorf("status %q, want ready", cfg.Status)
	}
}

func TestSetOnRequiresReady(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	live := filepath.Join(root, "demo")
	os.MkdirAll(live, 0o700)
	os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("x\n"), 0o600)
	goal := filepath.Join(root, "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	if _, err := s.Create("c", filepath.Join(live, "SKILL.md"), goal, "maximize-score"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOn("c", true); err == nil {
		t.Errorf("SetOn on a draft campaign succeeded")
	}
}

func TestSetOnAfterNoGainResetsCounter(t *testing.T) {
	f := newCampaign(t, func(c *Config) {
		c.Status, c.StopReason, c.Nights, c.NoGainNights = StatusOff, "no-gain", 3, 3
	})
	if err := f.store.SetOn(f.name, true); err != nil {
		t.Fatalf("SetOn: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusOn || cfg.NoGainNights != 0 || cfg.StopReason != "" {
		t.Errorf("after re-on: %q %d %q; want on, 0, empty", cfg.Status, cfg.NoGainNights, cfg.StopReason)
	}
}

func TestSetOnRefusedAfterMaxNights(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.Status, c.Nights = StatusOff, 7 })
	if err := f.store.SetOn(f.name, true); err == nil {
		t.Errorf("SetOn after max nights succeeded")
	}
}
