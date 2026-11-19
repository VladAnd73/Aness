package research

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if err := s.SetOn("c", true, testLock(t)); err == nil {
		t.Errorf("SetOn on a draft campaign succeeded")
	}
}

func TestSetOnAfterNoGainResetsCounter(t *testing.T) {
	f := newCampaign(t, func(c *Config) {
		c.Status, c.StopReason, c.Nights, c.NoGainNights = StatusOff, "no-gain", 3, 3
	})
	if err := f.store.SetOn(f.name, true, testLock(t)); err != nil {
		t.Fatalf("SetOn: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusOn || cfg.NoGainNights != 0 || cfg.StopReason != "" {
		t.Errorf("after re-on: %q %d %q; want on, 0, empty", cfg.Status, cfg.NoGainNights, cfg.StopReason)
	}
}

func TestSetOnRefusedAfterMaxNights(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.Status, c.Nights = StatusOff, 7 })
	if err := f.store.SetOn(f.name, true, testLock(t)); err == nil {
		t.Errorf("SetOn after max nights succeeded")
	}
}

func TestDefaultStoreResearchDirWinsOverXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("SPORE_RESEARCH_DIR", filepath.Join(dir, "store"))
	s, err := DefaultStore()
	if err != nil || s.Root != filepath.Join(dir, "store") {
		t.Errorf("DefaultStore = %q, %v; want SPORE_RESEARCH_DIR", s.Root, err)
	}
	t.Setenv("SPORE_RESEARCH_DIR", "")
	if s, _ := DefaultStore(); s.Root != filepath.Join(dir, "xdg", "spore", "research") {
		t.Errorf("DefaultStore without the override = %q", s.Root)
	}
}

func TestCreateFailureLeavesNoCampaignDir(t *testing.T) {
	s := Store{Root: t.TempDir()}
	live := filepath.Join(t.TempDir(), "demo")
	os.MkdirAll(filepath.Join(live, ".git"), 0o700)
	target := filepath.Join(live, "SKILL.md")
	os.WriteFile(target, []byte("x\n"), 0o600)
	goal := filepath.Join(t.TempDir(), "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	if _, err := s.Create("demo", target, goal, "maximize-score"); err == nil {
		t.Fatal("Create copied a nested .git")
	}
	if _, err := os.Lstat(s.Dir("demo")); err == nil {
		t.Errorf("failed Create left %s behind", s.Dir("demo"))
	}
	os.RemoveAll(filepath.Join(live, ".git"))
	if _, err := s.Create("demo", target, goal, "maximize-score"); err != nil {
		t.Errorf("retry after fixing the folder: %v", err)
	}
}

// waitsFor reports whether fn blocks while the campaign's config lock is
// held and finishes once it is released.
func waitsFor(t *testing.T, f fixture, fn func() error) {
	t.Helper()
	unlock, err := f.store.lockConfig(f.name)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		unlock()
		t.Fatalf("finished (err %v) while the config lock was held; want it to wait", err)
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	if err := <-done; err != nil {
		t.Errorf("after the lock was released: %v", err)
	}
}

func TestSetWaitsForConfigLock(t *testing.T) {
	f := newCampaign(t, nil)
	waitsFor(t, f, func() error { return f.store.Set(f.name, "max_nights", "10", testLock(t)) })
}

func TestStopWaitsForConfigLock(t *testing.T) {
	f := newCampaign(t, nil)
	d := f.deps(&fakeEditor{}, &fakeJudge{})
	waitsFor(t, f, func() error { return Stop(context.Background(), f.store, f.name, "operator", d) })
}

func TestAcceptWaitsForConfigLock(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	waitsFor(t, f, func() error { _, err := Accept(f.store, f.name, false); return err })
}

// on while a night or a stop holds the night lock would be overwritten
// by that run's own save; off must still work, it is how a night stops.
func TestSetOnRefusesOnWhileNightLockHeld(t *testing.T) {
	f := newCampaign(t, nil)
	if err := f.store.SetOn(f.name, false, f.lockPath()); err != nil {
		t.Fatal(err)
	}
	unlock, _, err := Lock(f.lockPath(), "other")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := f.store.SetOn(f.name, true, f.lockPath()); err == nil || !strings.Contains(err.Error(), "other") {
		t.Errorf("on under a held night lock = %v, want a refusal naming the holder", err)
	}
	if cfg := f.load(t); cfg.Status != StatusOff {
		t.Errorf("status %s, want still off", cfg.Status)
	}
}
