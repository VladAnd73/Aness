package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetMovesRepoToBestCommit(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.BestCommit = c.StartCommit })
	target := filepath.Join(f.repoDir(), "SKILL.md")
	if err := os.WriteFile(target, []byte("half an edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repoDir(), "stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "research.lock")
	commit, err := Reset(f.store, f.name, lock)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	cfg := f.load(t)
	if commit != cfg.BestCommit {
		t.Errorf("Reset returned %q, want the best commit %q", commit, cfg.BestCommit)
	}
	if clean, err := f.store.repo(f.name).Clean(); !clean || err != nil {
		t.Errorf("repo not clean after Reset: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != string(f.original) {
		t.Errorf("SKILL.md = %q, want the original", b)
	}
}

func TestResetRefusesWhileLockHeld(t *testing.T) {
	f := newCampaign(t, nil)
	if err := os.WriteFile(filepath.Join(f.repoDir(), "SKILL.md"), []byte("mid try\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "research.lock")
	unlock, _, err := Lock(lock, "other")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := Reset(f.store, f.name, lock); err == nil || !strings.Contains(err.Error(), "other") {
		t.Errorf("Reset under a held lock: err = %v, want a refusal naming the holder", err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.repoDir(), "SKILL.md")); string(b) != "mid try\n" {
		t.Errorf("Reset touched the repo while the lock was held")
	}
}
