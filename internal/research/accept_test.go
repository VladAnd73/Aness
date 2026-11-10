package research

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stoppedWinner leaves a stopped campaign whose winner passed holdout.
func stoppedWinner(t *testing.T, f fixture) {
	t.Helper()
	_, d := keptNight(t, f, func([]byte) float64 { return 70 })
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func winnerBytes(f fixture) []byte { return append(append([]byte{}, f.original...), "winner\n"...) }

func TestAcceptWritesWinnerWithBackup(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	res, err := Accept(f.store, f.name, false)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, winnerBytes(f)) {
		t.Errorf("live = %q", live)
	}
	if res.Backup != f.liveTarget+".research-demo.bak" {
		t.Errorf("backup path %q", res.Backup)
	}
	if b, _ := os.ReadFile(res.Backup); !bytes.Equal(b, f.original) {
		t.Errorf("backup = %q", b)
	}
	if f.load(t).Status != StatusAccepted {
		t.Errorf("status not accepted")
	}
}

func TestAcceptTwiceRefuses(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	if _, err := Accept(f.store, f.name, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f.liveTarget, []byte("operator edit\n"), 0o600)
	if _, err := Accept(f.store, f.name, true); err == nil || !strings.Contains(err.Error(), "already accepted") {
		t.Errorf("second Accept = %v, want already accepted", err)
	}
	if b, _ := os.ReadFile(f.liveTarget); string(b) != "operator edit\n" {
		t.Errorf("second accept wrote the live tree")
	}
}

func TestAcceptRefusesWhenLiveFolderChanged(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	os.WriteFile(filepath.Join(f.liveDir, "reference.md"), []byte("edited live\n"), 0o600)
	_, err := Accept(f.store, f.name, true)
	if err == nil || !strings.Contains(err.Error(), "reference.md") {
		t.Errorf("Accept = %v, want a refusal naming reference.md", err)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, f.original) {
		t.Errorf("refused accept wrote the live target")
	}
	if _, err := os.Stat(f.liveTarget + ".research-demo.bak"); err == nil {
		t.Errorf("refused accept wrote a backup")
	}
}

func TestAcceptRefusesWhenLiveFileAdded(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	os.WriteFile(filepath.Join(f.liveDir, "new-notes.md"), []byte("x\n"), 0o600)
	if _, err := Accept(f.store, f.name, true); err == nil || !strings.Contains(err.Error(), "new-notes.md") {
		t.Errorf("Accept = %v, want a refusal naming new-notes.md", err)
	}
}

func TestAcceptRefusesWithoutKeptChange(t *testing.T) {
	f := newCampaign(t, nil)
	if err := Stop(context.Background(), f.store, f.name, "operator", f.deps(&fakeEditor{}, &fakeJudge{})); err != nil {
		t.Fatal(err)
	}
	if _, err := Accept(f.store, f.name, true); err == nil || !strings.Contains(err.Error(), "no kept change") {
		t.Errorf("Accept = %v, want no kept change", err)
	}
}

func TestAcceptRefusesBeforeStop(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	keptNight(t, f, nil)
	if _, err := Accept(f.store, f.name, true); err == nil || !strings.Contains(err.Error(), "not stopped") {
		t.Errorf("Accept = %v, want not stopped", err)
	}
}

func TestAcceptRefusesExistingBackup(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	os.WriteFile(f.liveTarget+".research-demo.bak", []byte("old"), 0o600)
	if _, err := Accept(f.store, f.name, false); err == nil || !strings.Contains(err.Error(), "backup") {
		t.Errorf("Accept = %v, want a refusal naming the backup", err)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, f.original) {
		t.Errorf("live target changed")
	}
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// sporeRepoWinner builds a temp checkout declaring the spore module,
// with a stopped campaign on its skill whose winner passed holdout.
func sporeRepoWinner(t *testing.T) (top, target, headBefore string, store Store, f fixture) {
	t.Helper()
	root := t.TempDir()
	top = filepath.Join(root, "spore")
	live := filepath.Join(top, "skills", "demo")
	os.MkdirAll(live, 0o700)
	gitT(t, root, "init", "-q", "-b", "main", top)
	gitT(t, top, "config", "user.email", "t@localhost")
	gitT(t, top, "config", "user.name", "t")
	os.WriteFile(filepath.Join(top, "go.mod"), []byte("module "+sporeModule+"\n\ngo 1.25\n"), 0o600)
	original := []byte(strings.Repeat("b", 99) + "\n")
	target = filepath.Join(live, "SKILL.md")
	os.WriteFile(target, original, 0o600)
	gitT(t, top, "add", "-A")
	gitT(t, top, "commit", "-qm", "init")
	headBefore = gitT(t, top, "rev-parse", "HEAD")

	store = Store{Root: filepath.Join(root, "state")}
	goal := filepath.Join(root, "goal.md")
	os.WriteFile(goal, []byte("g\n"), 0o600)
	bin := filepath.Join(root, "bin")
	os.WriteFile(bin, []byte("b"), 0o700)
	if _, err := store.Create("demo", target, goal, "maximize-score"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveJudge("demo", "testdata/drafts", bin, []string{bin}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOn("demo", true); err != nil {
		t.Fatal(err)
	}
	cfg, _ := store.Load("demo")
	cfg.MaxTriesPerNight = 1
	store.Save(cfg)
	f = fixture{store: store, name: "demo", liveDir: live, liveTarget: target, original: original, notes: new([]string)}
	stoppedWinner(t, f)

	return
}

func TestAcceptInSporeRepoMakesBranch(t *testing.T) {
	top, target, headBefore, store, f := sporeRepoWinner(t)
	original := f.original
	res, err := Accept(store, "demo", false)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.Written != "research/demo:skills/demo/SKILL.md" {
		t.Errorf("res.Written = %q, want the branch and path", res.Written)
	}
	if res.Branch != "research/demo" || res.Backup != "" {
		t.Errorf("res = %+v, want branch research/demo and no backup", res)
	}
	if got := gitT(t, top, "show", "research/demo:skills/demo/SKILL.md"); got+"\n" != string(winnerBytes(f)) {
		t.Errorf("branch file = %q", got)
	}
	if files := gitT(t, top, "diff", "--name-only", headBefore, "research/demo"); files != "skills/demo/SKILL.md" {
		t.Errorf("branch commit touches %q, want only the target", files)
	}
	if msg := gitT(t, top, "log", "-1", "--format=%s", "research/demo"); msg != "research: demo winner for skills/demo/SKILL.md" {
		t.Errorf("commit subject %q", msg)
	}
	if gitT(t, top, "rev-parse", "HEAD") != headBefore || gitT(t, top, "branch", "--show-current") != "main" {
		t.Errorf("accept moved the operator's checkout")
	}
	if b, _ := os.ReadFile(target); !bytes.Equal(b, original) {
		t.Errorf("accept changed the operator's working tree")
	}
	if out := gitT(t, top, "worktree", "list"); strings.Count(out, "\n") != 0 {
		t.Errorf("a temporary worktree was left behind:\n%s", out)
	}
}

func TestAcceptPrunesTranscripts(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	stoppedWinner(t, f)
	tryDir := filepath.Join(f.store.Dir(f.name), "nights", "1", "try-1")
	os.WriteFile(filepath.Join(tryDir, "editor.jsonl"), []byte("{}\n"), 0o600)
	if _, err := Accept(f.store, f.name, false); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(tryDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "judge-1.json,judge-2.json" {
		t.Errorf("try dir after accept holds %v, want only the judge results", names)
	}
}

func TestAcceptBranchFailureLeavesNoBranch(t *testing.T) {
	top, _, _, store, _ := sporeRepoWinner(t)
	hooks := filepath.Join(top, ".git", "hooks")
	os.MkdirAll(hooks, 0o700)
	hook := filepath.Join(hooks, "commit-msg")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	if _, err := Accept(store, "demo", false); err == nil {
		t.Fatalf("Accept succeeded through a failing commit hook")
	}
	if out, _ := exec.Command("git", "-C", top, "branch", "--list", "research/demo").Output(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("a failed accept left branch research/demo behind")
	}
	os.Remove(hook)
	if _, err := Accept(store, "demo", false); err != nil {
		t.Errorf("retry after fixing the hook: %v", err)
	}
}

func TestAcceptBranchSurvivesStaleWorktree(t *testing.T) {
	top, _, _, store, f := sporeRepoWinner(t)
	wt := filepath.Join(store.Dir(f.name), "accept-worktree")
	gitT(t, top, "worktree", "add", "-q", "--detach", wt, "HEAD")
	os.RemoveAll(wt)
	if _, err := Accept(store, "demo", false); err != nil {
		t.Errorf("Accept after a killed run left a registered worktree: %v", err)
	}
}
