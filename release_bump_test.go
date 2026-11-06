package spore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const bumpScript = "scripts/bump-version"

// releaseFixture is a scratch stand-in for the GitHub fork: a bare
// origin plus a developer clone that lands "merged PRs" on main the way
// GitHub's merge button does (a --no-ff merge commit).
type releaseFixture struct {
	t      *testing.T
	root   string
	origin string
	dev    string
	env    []string
	runs   int
}

func newReleaseFixture(t *testing.T, version string, tagIt bool) *releaseFixture {
	t.Helper()
	root := t.TempDir()
	f := &releaseFixture{
		t:      t,
		root:   root,
		origin: filepath.Join(root, "origin.git"),
		dev:    filepath.Join(root, "dev"),
		env: append(hermeticEnv(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=dev", "GIT_AUTHOR_EMAIL=dev@example.invalid",
			"GIT_COMMITTER_NAME=dev", "GIT_COMMITTER_EMAIL=dev@example.invalid",
		),
	}
	f.git(root, "init", "-q", "--bare", "-b", "main", f.origin)
	f.git(root, "clone", "-q", f.origin, f.dev)
	f.git(f.dev, "checkout", "-q", "-b", "main")
	f.write("VERSION", version+"\n")
	f.write("CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- first note\n\n## 0.4.2 - 2026-05-06\n\n- old note\n")
	f.git(f.dev, "add", "-A")
	f.git(f.dev, "commit", "-q", "-m", "initial")
	if tagIt {
		f.git(f.dev, "tag", "-a", "v"+version, "-m", "v"+version)
	}
	f.git(f.dev, "push", "-q", "--follow-tags", "origin", "main")
	return f
}

func (f *releaseFixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *releaseFixture) write(rel, body string) {
	f.t.Helper()
	path := filepath.Join(f.dev, rel)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatalf("write %s: %v", rel, err)
	}
}

// merge lands one PR on origin/main as a merge commit and returns its SHA.
func (f *releaseFixture) merge(branch string, files map[string]string) string {
	f.t.Helper()
	f.git(f.dev, "fetch", "-q", "origin")
	f.git(f.dev, "checkout", "-q", "-B", branch, "origin/main")
	for rel, body := range files {
		f.write(rel, body)
	}
	f.git(f.dev, "add", "-A")
	f.git(f.dev, "commit", "-q", "-m", "work on "+branch)
	f.git(f.dev, "checkout", "-q", "-B", "main", "origin/main")
	f.git(f.dev, "merge", "-q", "--no-ff", "-m", "Merge pull request from "+branch, branch)
	f.git(f.dev, "push", "-q", "origin", "main")
	return f.git(f.dev, "rev-parse", "HEAD")
}

// checkout mirrors what actions/checkout leaves behind: a fresh clone
// of origin on main, with the bot identity the workflow configures.
func (f *releaseFixture) checkout(name string) string {
	f.t.Helper()
	dir := filepath.Join(f.root, name)
	f.git(f.root, "clone", "-q", "--branch", "main", f.origin, dir)
	f.git(dir, "config", "user.name", "github-actions[bot]")
	f.git(dir, "config", "user.email", "bot@example.invalid")
	return dir
}

// release runs the real script from a checkout exactly as the workflow
// does: no arguments, working directory at the checkout root.
func (f *releaseFixture) release(dir string) (string, int) {
	f.t.Helper()
	script, err := filepath.Abs(bumpScript)
	if err != nil {
		f.t.Fatalf("abs %s: %v", bumpScript, err)
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			f.t.Fatalf("run %s: %v\n%s", bumpScript, err, out)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

func (f *releaseFixture) releaseFresh() (string, int) {
	f.t.Helper()
	f.runs++
	return f.release(f.checkout(fmt.Sprintf("run-%d", f.runs)))
}

func (f *releaseFixture) originRev(rev string) string {
	f.t.Helper()
	return f.git(f.origin, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
}

func (f *releaseFixture) hasTag(tag string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/tags/"+tag)
	cmd.Dir = f.origin
	cmd.Env = f.env
	return cmd.Run() == nil
}

func (f *releaseFixture) originFile(rev, path string) string {
	f.t.Helper()
	return f.git(f.origin, "show", rev+":"+path)
}

func (f *releaseFixture) mustRelease() string {
	f.t.Helper()
	out, code := f.releaseFresh()
	if code != 0 {
		f.t.Fatalf("release exited %d\n%s", code, out)
	}
	return out
}

func TestBumpVersionReleasesTheNextPatchAfterAPlainMerge(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	merged := f.merge("feature", map[string]string{"feature.txt": "a\n"})

	f.mustRelease()

	if got := f.git(f.origin, "log", "-1", "--format=%s", "main"); got != "release: v1.0.1" {
		t.Fatalf("main tip subject = %q, want %q", got, "release: v1.0.1")
	}
	if got := f.git(f.origin, "rev-parse", "main^"); got != merged {
		t.Fatalf("release commit parent = %s, want the merge %s", got, merged)
	}
	if got := f.originRev("v1.0.1"); got != f.originRev("main") {
		t.Fatalf("v1.0.1 -> %s, want main tip %s", got, f.originRev("main"))
	}
	if got := f.originFile("v1.0.1", "VERSION"); got != "1.0.1" {
		t.Fatalf("VERSION at v1.0.1 = %q, want 1.0.1", got)
	}
}

func TestBumpVersionStampsTheUnreleasedChangelogSection(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	f.merge("feature", map[string]string{"feature.txt": "a\n"})

	f.mustRelease()

	today := time.Now().UTC().Format("2006-01-02")
	want := "# Changelog\n\n## Unreleased\n\n## 1.0.1 - " + today + "\n\n- first note\n\n## 0.4.2 - 2026-05-06"
	if got := f.originFile("v1.0.1", "CHANGELOG.md"); !strings.HasPrefix(got, want) {
		t.Fatalf("CHANGELOG at v1.0.1:\n%s\nwant prefix:\n%s", got, want)
	}
}

func TestBumpVersionLeavesAnEmptyUnreleasedSectionAlone(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	f.merge("feature", map[string]string{
		"CHANGELOG.md": "# Changelog\n\n## Unreleased\n\n## 0.4.2 - 2026-05-06\n\n- old note\n",
	})

	f.mustRelease()

	want := "# Changelog\n\n## Unreleased\n\n## 0.4.2 - 2026-05-06\n\n- old note"
	if got := f.originFile("v1.0.1", "CHANGELOG.md"); got != want {
		t.Fatalf("CHANGELOG at v1.0.1:\n%s\nwant:\n%s", got, want)
	}
}

func TestBumpVersionTagsAVersionTheMergeSetItself(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	merged := f.merge("minor", map[string]string{"VERSION": "1.1.0\n"})

	f.mustRelease()

	if got := f.originRev("main"); got != merged {
		t.Fatalf("main moved to %s, want it left at the merge %s (no bump commit)", got, merged)
	}
	if got := f.originRev("v1.1.0"); got != merged {
		t.Fatalf("v1.1.0 -> %s, want the merge %s", got, merged)
	}
	if f.hasTag("v1.1.1") {
		t.Fatal("v1.1.1 exists; a merge that sets VERSION must not also be bumped")
	}
}

func TestBumpVersionTagsTheFirstForkReleaseWithoutBumping(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", false)
	tip := f.originRev("main")

	f.mustRelease()

	if got := f.originRev("v1.0.0"); got != tip {
		t.Fatalf("v1.0.0 -> %s, want %s", got, tip)
	}
	if got := f.originRev("main"); got != tip {
		t.Fatalf("main moved to %s, want %s", got, tip)
	}
}

func TestBumpVersionIgnoresItsOwnReleaseCommit(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	f.merge("feature", map[string]string{"feature.txt": "a\n"})
	f.mustRelease()
	released := f.originRev("main")

	out := f.mustRelease()

	if got := f.originRev("main"); got != released {
		t.Fatalf("second run moved main from %s to %s\n%s", released, got, out)
	}
	if f.hasTag("v1.0.2") {
		t.Fatalf("second run created v1.0.2\n%s", out)
	}
	if !strings.Contains(out, "nothing to release") {
		t.Fatalf("second run output does not say there is nothing to release:\n%s", out)
	}
}

func TestBumpVersionRefusesToOverwriteAnExistingTag(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	stray := f.originRev("main")
	f.git(f.dev, "tag", "v1.0.1", stray)
	f.git(f.dev, "push", "-q", "origin", "v1.0.1")
	merged := f.merge("feature", map[string]string{"feature.txt": "a\n"})

	out, code := f.releaseFresh()

	if code == 0 {
		t.Fatalf("release succeeded over an existing tag:\n%s", out)
	}
	if !strings.Contains(out, "v1.0.1 already exists") {
		t.Fatalf("failure message does not name the existing tag:\n%s", out)
	}
	if got := f.originRev("v1.0.1"); got != stray {
		t.Fatalf("v1.0.1 moved from %s to %s", stray, got)
	}
	if got := f.originRev("main"); got != merged {
		t.Fatalf("main moved from %s to %s", merged, got)
	}
}

func TestBumpVersionRejectsAMalformedVersionFile(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	f.merge("broken", map[string]string{"VERSION": "1.0\n"})

	out, code := f.releaseFresh()

	if code == 0 {
		t.Fatalf("release succeeded with VERSION=1.0:\n%s", out)
	}
	if !strings.Contains(out, "X.Y.Z") {
		t.Fatalf("failure message does not say what VERSION must look like:\n%s", out)
	}
}

// A pre-push hook in the runner's checkout lands another merge on origin
// after the script fetched but before its push reaches the server, which
// is exactly the window a second PR merging seconds later hits.
func TestBumpVersionRetriesWhenMainMovesBeforeThePush(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)
	f.merge("first", map[string]string{"first.txt": "a\n"})
	f.git(f.dev, "fetch", "-q", "origin")
	f.git(f.dev, "checkout", "-q", "-B", "racer", "origin/main")
	f.write("racer.txt", "b\n")
	f.git(f.dev, "add", "-A")
	f.git(f.dev, "commit", "-q", "-m", "racing merge")
	racer := f.git(f.dev, "rev-parse", "HEAD")
	f.git(f.dev, "push", "-q", "origin", "racer")

	dir := f.checkout("runner")
	marker := filepath.Join(f.root, "raced")
	hook := "#!" + mustLookPath(t, "bash") + "\n" +
		"[ -e '" + marker + "' ] && exit 0\n" +
		"touch '" + marker + "'\n" +
		"git --git-dir='" + f.origin + "' update-ref refs/heads/main " + racer + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-push"), []byte(hook), 0o755); err != nil {
		t.Fatalf("write pre-push hook: %v", err)
	}

	out, code := f.release(dir)

	if code != 0 {
		t.Fatalf("release exited %d after main moved\n%s", code, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pre-push hook never ran, so the race was not exercised\n%s", out)
	}
	if got := f.git(f.origin, "rev-parse", "main^"); got != racer {
		t.Fatalf("release commit parent = %s, want the racing merge %s\n%s", got, racer, out)
	}
	if got := f.originRev("v1.0.1"); got != f.originRev("main") {
		t.Fatalf("v1.0.1 -> %s, want main tip %s", got, f.originRev("main"))
	}
	if got := f.originFile("v1.0.1", "racer.txt"); got != "b" {
		t.Fatalf("racer.txt at v1.0.1 = %q, want the racing change", got)
	}
	if f.hasTag("v1.0.2") {
		t.Fatalf("one release run created two tags\n%s", out)
	}
}

func TestBumpVersionReleasesTwoMergesInARow(t *testing.T) {
	f := newReleaseFixture(t, "1.0.0", true)

	f.merge("first", map[string]string{"first.txt": "a\n"})
	f.mustRelease()
	f.merge("second", map[string]string{"second.txt": "b\n"})
	f.mustRelease()

	for _, c := range []struct{ tag, version string }{{"v1.0.1", "1.0.1"}, {"v1.0.2", "1.0.2"}} {
		if !f.hasTag(c.tag) {
			t.Fatalf("origin has no %s", c.tag)
		}
		if got := f.originFile(c.tag, "VERSION"); got != c.version {
			t.Fatalf("VERSION at %s = %q, want %q", c.tag, got, c.version)
		}
	}
	if got := f.originRev("v1.0.2"); got != f.originRev("main") {
		t.Fatalf("v1.0.2 -> %s, want main tip %s", got, f.originRev("main"))
	}
	if out := f.git(f.origin, "ls-tree", "--name-only", "v1.0.1"); strings.Contains(out, "second.txt") {
		t.Fatal("v1.0.1 already holds the second merge")
	}
	if got := f.originFile("v1.0.2", "second.txt"); got != "b" {
		t.Fatalf("second.txt at v1.0.2 = %q", got)
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("locate %s: %v", name, err)
	}
	return p
}
