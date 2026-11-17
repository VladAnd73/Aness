package fleet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task"
)

const prActionLoop = `name = "pr"

[[steps]]
name = "open-pr"
kind = "action"
action = "push-and-open-pr"
label = "QA"

[[steps]]
name = "work"
kind = "agent"
role = "author"
output = "work/round-{round}.json"
shape = "json"
`

func prActionCtx(t *testing.T, f *mirrorFixture, src string, visit int) actionCtx {
	t.Helper()
	def := mustLoopDef(t, src)
	return actionCtx{ProjectRoot: f.root, TasksDir: f.tasksDir, Slug: f.slug, Def: def, Step: def.Steps[0], Visit: visit}
}

func prCreates(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, "pr create") {
			out = append(out, c)
		}
	}
	return out
}

func TestPushAndOpenPRFirstVisitOpens(t *testing.T) {
	f := newMirrorFixture(t, "")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	want := []string{
		"pr list --head wt/demo --state open --json number,url --limit 1",
		"pr create --base main --head wt/demo --title Demo widget --body-file - --assignee octo --label QA",
	}
	if got := f.calls(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls:\n got %q\nwant %q", got, want)
	}
	head := runGit(t, f.wt, "rev-parse", "HEAD")
	if f.bareHead(t) != head {
		t.Fatalf("branch not pushed to origin")
	}
	s := f.state(t)
	if s.PRNumber != 12 || s.PRURL != "https://github.com/acme/widget/pull/12" || s.PushedSHA != head {
		t.Fatalf("state = %+v", s)
	}
	log := f.log(t)
	if !strings.Contains(log, "TOKEN: ghs_app") || !strings.Contains(log, "Make the widget spin.") {
		t.Errorf("gh log lacks the app token or the spec summary:\n%s", log)
	}
	if strings.Contains(log, "Review #1") || strings.Contains(log, "Coordinator-only") {
		t.Errorf("PR body carries role-loop text or brief internals:\n%s", log)
	}
}

func TestPushAndOpenPRLaterVisitPushesOnly(t *testing.T) {
	f := newMirrorFixture(t, "")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("visit 1: %v", err)
	}
	before := len(f.calls(t))
	head := f.commit(t, "two")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 2)); err != nil {
		t.Fatalf("visit 2: %v", err)
	}
	if got := f.calls(t)[before:]; len(got) != 0 {
		t.Fatalf("visit 2 gh calls = %q, want none", got)
	}
	if f.bareHead(t) != head {
		t.Fatalf("remote ref did not move to %s", head)
	}
	if s := f.state(t); s.PRNumber != 12 || s.PushedSHA != head {
		t.Fatalf("state = %+v", s)
	}
}

// TestPushAndOpenPRAdoptsExistingPR: a PR already open for the branch
// (a lost github.json, or a racing drive that got there first) is
// adopted, never duplicated.
func TestPushAndOpenPRAdoptsExistingPR(t *testing.T) {
	f := newMirrorFixture(t, "")
	t.Setenv("FAKE_GH_OPEN_PR", `[{"number":7,"url":"https://github.com/acme/widget/pull/7"}]`)
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	want := []string{
		"pr list --head wt/demo --state open --json number,url --limit 1",
		"pr edit 7 --add-assignee octo --add-label QA",
	}
	if got := f.calls(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls:\n got %q\nwant %q", got, want)
	}
	if s := f.state(t); s.PRNumber != 7 || s.PRURL != "https://github.com/acme/widget/pull/7" {
		t.Fatalf("state = %+v", s)
	}
}

// TestPushAndOpenPRHeldLockOpensNothing: a concurrent drive holding the
// github state lock makes this visit fail (and retry) without touching
// GitHub.
func TestPushAndOpenPRHeldLockOpensNothing(t *testing.T) {
	f := newMirrorFixture(t, "")
	unlock, ok := flockBeside(githubStatePath(f.root, f.slug))
	if !ok {
		t.Fatal("lock")
	}
	defer unlock()
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err == nil {
		t.Fatal("action succeeded while another drive held the lock")
	}
	if c := f.calls(t); c != nil {
		t.Fatalf("gh calls = %q, want none", c)
	}
}

func TestPushAndOpenPRNoLabelWhenUnset(t *testing.T) {
	f := newMirrorFixture(t, "")
	src := strings.Replace(prActionLoop, "label = \"QA\"\n", "\n[steps.params]\ndraft = \"true\"\n", 1)
	if err := actionPushAndOpenPR(prActionCtx(t, f, src, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	creates := prCreates(f.calls(t))
	if len(creates) != 1 {
		t.Fatalf("pr create calls = %q", creates)
	}
	if strings.Contains(creates[0], "--label") {
		t.Errorf("pr create carries a label: %q", creates[0])
	}
	if !strings.HasPrefix(creates[0], "pr create --draft ") {
		t.Errorf("draft = \"true\" did not open a draft: %q", creates[0])
	}
	for _, c := range f.calls(t) {
		if strings.Contains(c, "merge") || strings.HasPrefix(c, "pr ready") {
			t.Errorf("forbidden gh call %q", c)
		}
	}
}

// TestPushAndOpenPRWithoutAppEscalatesOnce: no usable github.toml means
// no push, one coordinator notice naming the missing key across ticks,
// and the loop parked on the action step (no visit marker).
func TestPushAndOpenPRWithoutAppEscalatesOnce(t *testing.T) {
	f := newMirrorFixture(t, "")
	stateDir, err := task.StateDirForProject(f.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(stateDir, "github.toml")); err != nil {
		t.Fatal(err)
	}
	def := mustLoopDef(t, prActionLoop)
	for i := 0; i < 3; i++ {
		st, _, _, err := advanceLoop(f.root, f.tasksDir, f.slug, def)
		if err == nil {
			t.Fatalf("tick %d: no error from a push-and-open-pr without config", i)
		}
		if st.Step != "open-pr" {
			t.Fatalf("tick %d: cursor = %q, want parked on open-pr", i, st.Step)
		}
	}
	if _, err := os.Stat(actionMarker(f.root, f.slug, "open-pr", 1)); !os.IsNotExist(err) {
		t.Fatalf("visit marker written for a failed action (stat err %v)", err)
	}
	if c := f.calls(t); c != nil {
		t.Fatalf("gh calls = %q, want none", c)
	}
	if out, err := runGitOut(f.bare, "rev-parse", "--verify", "-q", "refs/heads/wt/demo"); err == nil {
		t.Fatalf("branch pushed without config: %s", out)
	}
	envs := readInboxEnvelopes(t, f.inbox)
	if len(envs) != 1 || !strings.Contains(envs[0].Body, "demo:") || !strings.Contains(envs[0].Body, "app_id") {
		t.Fatalf("envelopes = %+v, want one naming app_id", envs)
	}
}

func runGitOut(dir string, args ...string) (string, error) {
	m := &githubMirror{worktree: dir, env: os.Environ()}
	return m.git(args...)
}

// gitConfigs lists every config scope git reads in dir, so a test can
// prove an action left remote config untouched.
func gitConfigs(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, dir, "config", "--list", "--show-origin")
}

// TestPushAndOpenPRSSHOriginPushesOverHTTPS: an ssh origin (which the
// app token cannot push to) is pushed through its https form, and no
// remote config is rewritten to get there.
func TestPushAndOpenPRSSHOriginPushesOverHTTPS(t *testing.T) {
	f := newMirrorFixture(t, "")
	runGit(t, f.root, "remote", "set-url", "origin", "git@github.com:acme/widget.git")
	t.Setenv("GIT_SSH_COMMAND", "false")
	before, beforeWT := gitConfigs(t, f.root), gitConfigs(t, f.wt)
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	if f.bareHead(t) != runGit(t, f.wt, "rev-parse", "HEAD") {
		t.Fatalf("branch not pushed over https")
	}
	if !strings.Contains(f.log(t), "REPO: github.com/acme/widget\n") {
		t.Errorf("gh not pinned to origin's repo:\n%s", f.log(t))
	}
	if s := f.state(t); s.PRNumber != 12 {
		t.Fatalf("state = %+v", s)
	}
	if after := gitConfigs(t, f.root); after != before {
		t.Errorf("repo config changed:\nbefore %s\nafter  %s", before, after)
	}
	if after := gitConfigs(t, f.wt); after != beforeWT {
		t.Errorf("worktree config changed:\nbefore %s\nafter  %s", beforeWT, after)
	}
	if got := runGit(t, f.root, "config", "--get", "remote.origin.url"); got != "git@github.com:acme/widget.git" {
		t.Errorf("origin url = %q", got)
	}
}

// TestPushAndOpenPRNonGitHubSSHOriginErrors: an ssh host with no known
// https form fails clearly, naming the remote, before any push.
func TestPushAndOpenPRNonGitHubSSHOriginErrors(t *testing.T) {
	f := newMirrorFixture(t, "")
	runGit(t, f.root, "remote", "set-url", "origin", "git@gitlab.com:acme/widget.git")
	t.Setenv("GIT_SSH_COMMAND", "false")
	err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1))
	if err == nil || !strings.Contains(err.Error(), "git@gitlab.com:acme/widget.git") {
		t.Fatalf("err = %v, want one naming the remote", err)
	}
	if c := f.calls(t); c != nil {
		t.Fatalf("gh ran: %v", c)
	}
}

// TestPushAndOpenPRSecondHTTPSURLPushesOnlyThere: a worktree-style
// workaround leaves origin with an ssh url plus an added https url. A
// push by remote name would try every url and die on the ssh one; the
// action pushes to the https url alone.
func TestPushAndOpenPRSecondHTTPSURLPushesOnlyThere(t *testing.T) {
	f := newMirrorFixture(t, "")
	runGit(t, f.root, "remote", "set-url", "origin", "git@github.com:acme/widget.git")
	runGit(t, f.root, "config", "--add", "remote.origin.url", "https://github.com/acme/widget.git")
	t.Setenv("GIT_SSH_COMMAND", "false")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	if f.bareHead(t) != runGit(t, f.wt, "rev-parse", "HEAD") {
		t.Fatalf("branch not pushed over https")
	}
}

// prBodies returns every --body-file payload gh received.
func prBodies(t *testing.T, f *mirrorFixture) []string {
	t.Helper()
	var out []string
	for _, chunk := range strings.Split(f.log(t), "BODY:\n")[1:] {
		body, _, _ := strings.Cut(chunk, "END\n")
		out = append(out, body)
	}
	return out
}

func TestPushAndOpenPRFooterOffBodyIsSummaryOnly(t *testing.T) {
	f := newMirrorFixture(t, "")
	if err := actionPushAndOpenPR(prActionCtx(t, f, "pr_footer = false\n"+prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	if got := prBodies(t, f); !reflect.DeepEqual(got, []string{"Make the widget spin.\n"}) {
		t.Fatalf("bodies = %q, want the summary only", got)
	}
}

func TestPushAndOpenPRFooterDefaultUnchanged(t *testing.T) {
	f := newMirrorFixture(t, "")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	want := "Make the widget spin.\n\nDispatched by @octo. Opened by spore's `pr` loop. @octo gives the only approval and merges.\n"
	if got := prBodies(t, f); !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("bodies = %q, want %q", got, want)
	}
}

// TestGitHubMirrorFooterOff: a github_mirror loop with pr_footer =
// false opens its draft PR with the summary only.
func TestGitHubMirrorFooterOff(t *testing.T) {
	f := newMirrorFixture(t, "draft")
	writeEng(1)(t, f.root, f.slug)
	snap, err := DeriveSnapshot(f.root, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	mirrorGitHub(f.root, f.tasksDir, f.slug, snap, false)
	if got := prBodies(t, f); len(got) == 0 || got[0] != "Make the widget spin.\n" {
		t.Fatalf("bodies = %q, want the summary only first", got)
	}
}

// trapSSH points git's ssh at a script that records being run, so a
// test can prove a push never left over ssh.
func trapSSH(t *testing.T) string {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "ssh-ran")
	script := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", script)
	return marker
}

// TestPushAndOpenPRRefusesRewriteToSSH: an operator's global
// insteadOf or pushInsteadOf that turns github https into ssh must
// not carry the app push over ssh under the operator's own key.
func TestPushAndOpenPRRefusesRewriteToSSH(t *testing.T) {
	for _, key := range []string{"insteadOf", "pushInsteadOf"} {
		t.Run(key, func(t *testing.T) {
			f := newMirrorFixture(t, "")
			key0, _, _ := strings.Cut(runGit(t, f.root, "config", "--get-regexp", `^url\..*\.insteadof$`), " ")
			runGit(t, f.root, "config", "--remove-section", strings.TrimSuffix(key0, ".insteadof"))
			global := filepath.Join(t.TempDir(), "gitconfig")
			if err := os.WriteFile(global, []byte("[url \"git@github.com:\"]\n\t"+key+" = https://github.com/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			marker := trapSSH(t)
			err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1))
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatalf("the app push went over ssh (err = %v)", err)
			}
			if err == nil || !strings.Contains(err.Error(), "https") {
				t.Fatalf("err = %v, want a refusal naming https", err)
			}
		})
	}
}

// TestPushAndOpenPRHonorsPushURL: a fork workflow (url = upstream,
// pushurl = fork) pushes to the fork and pins gh there, as `git push
// origin` did.
func TestPushAndOpenPRHonorsPushURL(t *testing.T) {
	f := newMirrorFixture(t, "")
	runGit(t, f.root, "remote", "set-url", "origin", "https://github.com/upstream/widget.git")
	runGit(t, f.root, "config", "remote.origin.pushurl", "https://github.com/acme/widget.git")
	if err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1)); err != nil {
		t.Fatalf("action: %v", err)
	}
	if f.bareHead(t) != runGit(t, f.wt, "rev-parse", "HEAD") {
		t.Fatalf("branch not pushed to the pushurl")
	}
	if !strings.Contains(f.log(t), "REPO: github.com/acme/widget\n") {
		t.Errorf("gh not pinned to the push target:\n%s", f.log(t))
	}
}

// TestPushAndOpenPRSSHAliasErrorNamesPushURL: an ssh config alias has
// no knowable https form; the error says to set an https pushurl.
func TestPushAndOpenPRSSHAliasErrorNamesPushURL(t *testing.T) {
	f := newMirrorFixture(t, "")
	runGit(t, f.root, "remote", "set-url", "origin", "git@github-work:acme/widget.git")
	trapSSH(t)
	err := actionPushAndOpenPR(prActionCtx(t, f, prActionLoop, 1))
	if err == nil || !strings.Contains(err.Error(), "remote.origin.pushurl") {
		t.Fatalf("err = %v, want one telling to set remote.origin.pushurl", err)
	}
}
