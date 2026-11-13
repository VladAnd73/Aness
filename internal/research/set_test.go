package research

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSetRebuildsDefaultJudgeArgv(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.Status = StatusReady })
	binary := filepath.Join(t.TempDir(), "spore")
	if err := os.WriteFile(binary, []byte("judge\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApproveJudge(f.name, "testdata/drafts", binary, nil); err != nil {
		t.Fatalf("ApproveJudge: %v", err)
	}
	for _, kv := range [][2]string{{"model", "haiku"}, {"repeats", "1"}, {"run_timeout", "2m"}} {
		if err := f.store.Set(f.name, kv[0], kv[1], testLock(t)); err != nil {
			t.Fatalf("Set %s: %v", kv[0], err)
		}
	}
	cfg := f.load(t)
	if cfg.Model != "haiku" || cfg.Repeats != 1 || cfg.RunTimeout != 2*time.Minute {
		t.Errorf("cfg model=%q repeats=%d timeout=%v, want haiku 1 2m", cfg.Model, cfg.Repeats, cfg.RunTimeout)
	}
	argv := strings.Join(cfg.JudgeArgv, " ")
	for _, want := range []string{"--model haiku", "--repeats 1", "--timeout 2m0s"} {
		if !strings.Contains(argv, want) {
			t.Errorf("judge argv %q lacks %q", argv, want)
		}
	}
}

func TestSetKeepsCustomJudgeArgv(t *testing.T) {
	f := newCampaign(t, nil)
	before := f.load(t).JudgeArgv
	if err := f.store.Set(f.name, "model", "haiku", testLock(t)); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := f.load(t).JudgeArgv; !slices.Equal(got, before) {
		t.Errorf("custom judge argv changed: %v -> %v", before, got)
	}
}

func TestSetRefusesBadInput(t *testing.T) {
	f := newCampaign(t, nil)
	for _, kv := range [][2]string{
		{"best_score", "100"}, {"bogus", "1"}, {"repeats", "abc"}, {"repeats", "0"},
		{"isolation", "docker"}, {"run_timeout", "soon"}, {"max_hours_per_night", "-1"},
	} {
		if err := f.store.Set(f.name, kv[0], kv[1], testLock(t)); err == nil {
			t.Errorf("Set %s=%s: want an error", kv[0], kv[1])
		}
	}
	if err := f.store.Set(f.name, "max_tries_per_night", "4", testLock(t)); err != nil {
		t.Errorf("Set max_tries_per_night: %v", err)
	}
}

func TestSetRefusesStoppedCampaign(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.Status = StatusStopped })
	if err := f.store.Set(f.name, "model", "haiku", testLock(t)); err == nil {
		t.Errorf("Set on a stopped campaign: want an error")
	}
}

func TestSetRefusesWhileNightRuns(t *testing.T) {
	f := newCampaign(t, nil)
	lock := filepath.Join(t.TempDir(), "research.lock")
	unlock, _, err := Lock(lock, f.name)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := f.store.Set(f.name, "model", "haiku", lock); err == nil || !strings.Contains(err.Error(), f.name) {
		t.Errorf("Set while the lock is held: err = %v, want a refusal naming %s", err, f.name)
	}
}

func testLock(t *testing.T) string { return filepath.Join(t.TempDir(), "research.lock") }
