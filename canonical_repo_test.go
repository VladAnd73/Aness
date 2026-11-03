package spore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The bracket keeps the pre-2026-11-03 repo name out of `git grep`.
var staleRepoURL = regexp.MustCompile(`(?:marketertechnologies|versality)/spore|Agentic[H]arness`)

// moduleRef is the Go module path, not a URL: imports, `go build`
// targets and the buildCommit ldflag keep it after the repo moved.
var moduleRef = regexp.MustCompile(`"github\.com/versality/spore(?:/[^"\s]*)?"|=github\.com/versality/spore\.buildCommit`)

// historicalRepoFiles record past events or other repos and are left
// as written.
var historicalRepoFiles = map[string]bool{
	"CHANGELOG.md":                    true,
	"docs/todo/host-shims-via-nix.md": true,
}

// staleRepoHits scans the files git tracks under root. Build output,
// a `nix build` result link and ignored scratch files are not the repo.
func staleRepoHits(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" || historicalRepoFiles[rel] || rel == "canonical_repo_test.go" || rel == "go.mod" {
			continue
		}
		p := filepath.Join(root, rel)
		// Symlinks, submodules and files deleted in the work tree.
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(raw), "\n") {
			stripped := moduleRef.ReplaceAllString(line, "")
			if staleRepoURL.MatchString(stripped) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
	}
	return hits, nil
}

func TestNoStaleRepoURLs(t *testing.T) {
	// The nix go-test check runs on a source copy without .git; the
	// `go test` in a checkout (just check, CI) covers it.
	if _, err := os.Stat(".git"); err != nil {
		t.Skip("not a git checkout")
	}
	hits, err := staleRepoHits(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		t.Errorf("stale repo URL, use VladAnd73/Aness: %s", h)
	}
}

// Build output, a `nix build` result link and git-ignored scratch files
// are not the repo: only tracked files count.
func TestStaleRepoScanReadsOnlyTrackedFiles(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := "see github.com/marketertechnologies/spore\n"
	git("init", "-q")
	write(".gitignore", "/build/\n/result\ndocs/todo/*-plan.md\n")
	write("README.md", "clean\n")
	git("add", ".")
	git("commit", "-qm", "init")
	write("build/spore", old)
	write("docs/todo/x-plan.md", old)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "result")); err != nil {
		t.Fatal(err)
	}

	if hits, err := staleRepoHits(root); err != nil || len(hits) > 0 {
		t.Fatalf("untracked files flagged: hits=%v err=%v", hits, err)
	}

	write("docs/guide.md", old)
	git("add", "docs/guide.md")
	if hits, err := staleRepoHits(root); err != nil || len(hits) != 1 || !strings.HasPrefix(hits[0], "docs/guide.md:1:") {
		t.Fatalf("tracked stale URL not reported once: hits=%v err=%v", hits, err)
	}
}
