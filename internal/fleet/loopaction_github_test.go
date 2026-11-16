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
	unlock, ok := lockGitHubState(githubStatePath(f.root, f.slug))
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
