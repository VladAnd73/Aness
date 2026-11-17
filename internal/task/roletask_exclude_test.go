package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func excludeLines(t *testing.T, gitDir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == ".spore/" {
			out = append(out, l)
		}
	}
	return out
}

func TestEnsureRoleTaskDirHidesSporeFromGitStatus(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	dir, err := EnsureRoleTaskDir(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := gitIn(t, root, "status", "--porcelain", "--untracked-files=all"); strings.Contains(out, ".spore/") {
		t.Fatalf("git status shows .spore/:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("a .gitignore was written: %v", err)
	}
}

func TestEnsureRoleTaskDirExcludesOnce(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	for _, slug := range []string{"demo", "demo", "other"} {
		if _, err := EnsureRoleTaskDir(root, slug); err != nil {
			t.Fatal(err)
		}
	}
	if got := excludeLines(t, filepath.Join(root, ".git")); len(got) != 1 {
		t.Fatalf(".spore/ lines = %q, want exactly one", got)
	}
}

func TestEnsureRoleTaskDirExcludeKeepsExistingLines(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	path := filepath.Join(root, ".git", "info", "exclude")
	if err := os.WriteFile(path, []byte("*.log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureRoleTaskDir(root, "demo"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "*.log\n.spore/\n" {
		t.Fatalf("exclude = %q", b)
	}
}

func TestEnsureRoleTaskDirFromWorktreeExcludesInMainRepo(t *testing.T) {
	main := t.TempDir()
	gitIn(t, main, "init", "-q")
	gitIn(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, main, "worktree", "add", "-q", "-b", "wt/demo", wt)
	if _, err := EnsureRoleTaskDir(wt, "demo"); err != nil {
		t.Fatal(err)
	}
	if got := excludeLines(t, filepath.Join(main, ".git")); len(got) != 1 {
		t.Fatalf("main repo .spore/ lines = %q, want one", got)
	}
	if out := gitIn(t, wt, "status", "--porcelain", "--untracked-files=all"); strings.Contains(out, ".spore/") {
		t.Fatalf("worktree git status shows .spore/:\n%s", out)
	}
}

func TestEnsureRoleTaskDirOutsideGitStillWorks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	if _, err := EnsureRoleTaskDir(root, "demo"); err != nil {
		t.Fatalf("non-git root: %v", err)
	}
}
