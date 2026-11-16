package fleet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

// recordWakeStems wraps the fake wake seam so a test also sees the
// marker stem of each delivered wake, keyed by session.
func recordWakeStems(t *testing.T) map[string][]string {
	t.Helper()
	stems := map[string][]string{}
	inner := wakeRole
	wakeRole = func(spec RoleSpawnSpec, stem, msg string) (wakeOutcome, error) {
		s, err := spec.SessionName()
		if err == nil {
			stems[s] = append(stems[s], stem)
		}
		return inner(spec, stem, msg)
	}
	t.Cleanup(func() { wakeRole = inner })
	return stems
}

// newSrcLoopFixture is newLoopFixture with the loop file given inline.
func newSrcLoopFixture(t *testing.T, name, src string, roles ...string) *loopFixture {
	t.Helper()
	f := newLoopFixture(t, name, "trio.toml", roles...)
	p := filepath.Join(f.root, "bootstrap", "loops", name+".toml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// newDevFixture runs the bundled dev loop: no project override.
func newDevFixture(t *testing.T, loopKey string) *loopFixture {
	t.Helper()
	f := newLoopFixture(t, "unused", "trio.toml")
	if err := os.RemoveAll(filepath.Join(f.root, "bootstrap")); err != nil {
		t.Fatal(err)
	}
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", loopKey)
	return f
}

func TestDriveLoopDevMatchesUpstreamWakeText(t *testing.T) {
	f := newDevFixture(t, "dev")
	stems := recordWakeStems(t)
	eng, revA, revB := f.session("engineer"), f.session("reviewer-A"), f.session("reviewer-B")
	A, B := task.ReviewerA, task.ReviewerB

	expectLastWake := func(label, session, wantStem, wantMsg string) {
		t.Helper()
		w := f.panes.wakesTo(session)
		if len(w) == 0 || w[len(w)-1] != wantMsg {
			t.Errorf("%s: wakes to %s = %q, want last %q", label, session, w, wantMsg)
		}
		s := stems[session]
		if len(s) == 0 || s[len(s)-1] != wantStem {
			t.Errorf("%s: stems to %s = %q, want last %q", label, session, s, wantStem)
		}
	}
	engMsg := func(snap Snapshot) string {
		return fmt.Sprintf("Reviewer %s requested changes at round %d. Read %s and start the revision.",
			snap.CurrentReviewer, snap.ReviewerRound, task.ReviewPath(f.root, f.slug, snap.CurrentReviewer, snap.ReviewerRound))
	}
	revMsg := func(snap Snapshot) string {
		return fmt.Sprintf("Engineer responded to your round %d verdict. Read %s and review round %d.",
			snap.ReviewerRound, task.EngineerResponsePath(f.root, f.slug, snap.EngineerRound), snap.ReviewerRound+1)
	}

	writeDevStep(t, f.root, f.slug, devStep{who: "eng", round: 1})
	f.tick(t)
	writeDevStep(t, f.root, f.slug, devStep{who: "A", round: 1, verdict: task.VerdictRequestChanges})
	f.tick(t)
	snap := Snapshot{CurrentReviewer: A, ReviewerRound: 1, EngineerRound: 1}
	expectLastWake("A round 1 request_changes", eng, engineerWakeMarkerStem(snap), engMsg(snap))

	writeDevStep(t, f.root, f.slug, devStep{who: "eng", round: 2})
	f.tick(t)
	snap = Snapshot{CurrentReviewer: A, ReviewerRound: 1, EngineerRound: 2}
	expectLastWake("engineer round 2", revA, reviewerWakeMarkerStem(snap), revMsg(snap))

	writeDevStep(t, f.root, f.slug, devStep{who: "A", round: 2, verdict: task.VerdictApprove})
	f.tick(t)
	writeDevStep(t, f.root, f.slug, devStep{who: "B", round: 1, verdict: task.VerdictRequestChanges})
	f.tick(t)
	snap = Snapshot{CurrentReviewer: B, ReviewerRound: 1, EngineerRound: 2}
	expectLastWake("B round 1 request_changes", eng, engineerWakeMarkerStem(snap), engMsg(snap))

	writeDevStep(t, f.root, f.slug, devStep{who: "eng", round: 3})
	f.tick(t)
	snap = Snapshot{CurrentReviewer: B, ReviewerRound: 1, EngineerRound: 3}
	expectLastWake("engineer round 3", revB, reviewerWakeMarkerStem(snap), revMsg(snap))

	if n := len(f.panes.wakesTo(revA)); n != 1 {
		t.Errorf("reviewer A got %d wakes, want 1 (round 0 and its forward exit wake nothing)", n)
	}
}

func TestDriveLoopReapsNonPersistOnForwardExit(t *testing.T) {
	f := newDevFixture(t, "role")
	eng, revA, revB := f.session("engineer"), f.session("reviewer-A"), f.session("reviewer-B")

	writeDevStep(t, f.root, f.slug, devStep{who: "eng", round: 1})
	f.tick(t)
	writeDevStep(t, f.root, f.slug, devStep{who: "A", round: 1, verdict: task.VerdictRequestChanges})
	if st := f.tick(t); st.Phase != string(PhaseEngineerReviseA) {
		t.Fatalf("state = %+v, want phase %s", st, PhaseEngineerReviseA)
	}
	if len(f.panes.kills) != 0 {
		t.Errorf("serving A's return killed %v; A must stay alive for the re-review", f.panes.kills)
	}

	writeDevStep(t, f.root, f.slug, devStep{who: "eng", round: 2})
	f.tick(t)
	writeDevStep(t, f.root, f.slug, devStep{who: "A", round: 2, verdict: task.VerdictApprove})
	f.tick(t)
	if want := []string{revA}; !equalSlices(f.panes.kills, want) {
		t.Errorf("entering review-B: kills = %v, want %v (A reaped, engineer persists)", f.panes.kills, want)
	}
	if _, ok := f.panes.panes[revB]; !ok {
		t.Errorf("review-B: reviewer B pane not up; spawns = %v", f.panes.spawns)
	}

	writeDevStep(t, f.root, f.slug, devStep{who: "B", round: 1, verdict: task.VerdictApprove})
	if st := f.tick(t); st.Terminal != "done" {
		t.Fatalf("state = %+v, want done", st)
	}
	if countContaining(f.panes.kills, eng) != 1 || countContaining(f.panes.kills, revB) != 1 {
		t.Errorf("done: kills = %v, want engineer and reviewer B reaped once each", f.panes.kills)
	}
	if len(f.panes.panes) != 0 {
		t.Errorf("done: panes still up: %v", f.panes.panes)
	}
}

const twiceLoop = `name = "twice"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
title = "Alpha"
persist = true
output = "a/round-{round}.json"
shape = "json"
required = ["result"]
skills = ["tdd", "lint"]

[[steps]]
name = "n"
kind = "action"
action = "notify-coordinator"
message = "n visited"

[[steps]]
name = "c"
kind = "agent"
role = "beta"
title = "Checker"
output = "c/round-{round}.json"
shape = "verdict"
max_rounds = 3

[[steps.next]]
on = "request_changes"
to = "a"

[[steps.next]]
on = "approve"
to = "done"
`

func TestDriveLoopActionRunsOncePerVisit(t *testing.T) {
	f := newSrcLoopFixture(t, "twice", twiceLoop, "alpha", "beta")
	visits := func() int { return countContaining(inboxBodies(t, f.inbox), "n visited") }

	f.write(t, "a/round-1.json", `{"result":"x"}`)
	if st := f.tick(t); st.Step != "c" {
		t.Fatalf("after a round 1: state = %+v, want Step c (the action runs in the same tick)", st)
	}
	f.tick(t)
	if n := visits(); n != 1 {
		t.Errorf("first visit: action ran %d times over two ticks, want 1", n)
	}
	if !f.exists("state/action-n-v1") {
		t.Errorf("no state/action-n-v1 marker; markers = %v", f.stateMarkers(""))
	}

	f.write(t, "c/round-1.json", verdictJSON("c", task.VerdictRequestChanges))
	if st := f.tick(t); st.Step != "a" {
		t.Fatalf("after c request_changes: state = %+v, want Step a", st)
	}
	if n := visits(); n != 1 {
		t.Errorf("back at a: action ran %d times, want still 1", n)
	}
	f.write(t, "a/round-2.json", `{"result":"y"}`)
	f.tick(t)
	f.tick(t)
	if n := visits(); n != 2 {
		t.Errorf("second visit: action ran %d times in total, want 2", n)
	}
	if !f.exists("state/action-n-v2") {
		t.Errorf("no state/action-n-v2 marker; markers = %v", f.stateMarkers(""))
	}
}

// A plain (non-return) jump back wakes the step it lands on with the
// verdict, and re-entering the sender wakes it with the new output.
func TestDriveLoopWakesOnBackwardJump(t *testing.T) {
	f := newSrcLoopFixture(t, "twice", twiceLoop, "alpha", "beta")
	sa, sc := f.session("a"), f.session("c")

	f.write(t, "a/round-1.json", `{"result":"x"}`)
	f.tick(t)
	f.write(t, "c/round-1.json", verdictJSON("c", task.VerdictRequestChanges))
	f.tick(t)
	wantA := fmt.Sprintf("Checker requested changes at round 1. Read %s and start the revision. Use skills: tdd, lint.",
		f.path("c/round-1.json"))
	if w := f.panes.wakesTo(sa); !equalSlices(w, []string{wantA}) {
		t.Errorf("wakes to a = %q, want %q", w, wantA)
	}
	if len(f.panes.kills) != 0 {
		t.Errorf("jump back to a killed %v; c keeps its pane for the re-check", f.panes.kills)
	}

	f.write(t, "a/round-2.json", `{"result":"y"}`)
	f.tick(t)
	wantC := fmt.Sprintf("Alpha responded to your round 1 verdict. Read %s and review round 2.", f.path("a/round-2.json"))
	if w := f.panes.wakesTo(sc); !equalSlices(w, []string{wantC}) {
		t.Errorf("wakes to c = %q, want %q", w, wantC)
	}
}

func TestDriveLoopEscalationUsesNameWithoutLabel(t *testing.T) {
	f := newLoopFixture(t, "revise", "revise.toml", "author", "reviewer")
	f.write(t, "author/round-1.json", `{"notes":"1"}`)
	f.write(t, "review/round-1.json", verdictJSON("review", task.VerdictRequestChanges))
	f.write(t, "author/round-2.json", `{"notes":"2"}`)
	f.write(t, "review/round-2.json", verdictJSON("review", task.VerdictRequestChanges))
	f.tick(t)
	f.tick(t)
	if n := countContaining(inboxBodies(t, f.inbox), "demo: escalated (review, round 2)"); n != 1 {
		t.Errorf("escalation notices = %q, want one %q", inboxBodies(t, f.inbox), "demo: escalated (review, round 2)")
	}
	if len(f.panes.spawns) != 0 || len(f.panes.kills) != 0 {
		t.Errorf("escalated loop touched panes: spawns %v kills %v", f.panes.spawns, f.panes.kills)
	}
}

func TestDriveLoopRefusesTaskWithoutLoop(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	writeTask(t, f.tasks, f.slug, "active")
	if _, err := DriveLoop(f.root, f.tasks, f.slug); err == nil {
		t.Fatalf("DriveLoop accepted a task without loop:")
	}
	if len(f.panes.spawns) != 0 {
		t.Errorf("refused drive spawned %v", f.panes.spawns)
	}
	if _, err := os.Stat(loopTaskDir(f.root, f.slug)); !os.IsNotExist(err) {
		t.Errorf("refused drive created the task tree: %v", err)
	}
}

func TestStepSpecSlotThenOverride(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := newGitRepoFor(t, "demo-project")
	writeSporeTOML(t, root, "[fleet.reviewers.A]\nmode = \"code-review\"\neffort = \"high\"\n")
	def, err := ParseLoopDef("x.toml", []byte(`name = "x"

[[steps]]
name = "plain"
kind = "agent"
role = "reviewer"
label = "A"
slot = "A"
output = "p/round-{round}.json"
shape = "verdict"
skills = ["one", "two"]

[[steps]]
name = "effort"
kind = "agent"
role = "reviewer"
slot = "A"
effort = "low"
output = "e/round-{round}.json"
shape = "verdict"

[[steps]]
name = "mode"
kind = "agent"
role = "reviewer"
slot = "A"
mode = "fleet"
output = "m/round-{round}.json"
shape = "verdict"

[[steps]]
name = "noslot"
kind = "agent"
role = "reviewer"
output = "n/round-{round}.json"
shape = "verdict"
`))
	if err != nil {
		t.Fatal(err)
	}
	args := func(step string) []string {
		t.Helper()
		a, err := StepSpec(root, "feat", def, def.Steps[def.StepIndex(step)]).TmuxArgs()
		if err != nil {
			t.Fatalf("%s TmuxArgs: %v", step, err)
		}
		return a
	}
	cases := []struct {
		step, mode, effort, body string
	}{
		{"plain", "code-review", "high", "reviewer-code-review.md"},
		{"effort", "code-review", "low", "reviewer-code-review.md"},
		{"mode", "fleet", "", "reviewer.md"},
		{"noslot", "", "", "reviewer.md"},
	}
	for _, c := range cases {
		a := args(c.step)
		if got := envValue(a, "SPORE_REVIEWER_MODE"); got != c.mode {
			t.Errorf("%s: SPORE_REVIEWER_MODE = %q, want %q", c.step, got, c.mode)
		}
		if got := envValue(a, "SPORE_REVIEW_EFFORT"); got != c.effort {
			t.Errorf("%s: SPORE_REVIEW_EFFORT = %q, want %q", c.step, got, c.effort)
		}
		if cmd := a[len(a)-1]; !strings.Contains(cmd, "/"+c.body+"'") {
			t.Errorf("%s: command %q does not read body %s", c.step, cmd, c.body)
		}
		if got := envValue(a, "SPORE_STEP"); got != c.step {
			t.Errorf("%s: SPORE_STEP = %q", c.step, got)
		}
		if got := envValue(a, "SPORE_LOOP"); got != "x" {
			t.Errorf("%s: SPORE_LOOP = %q, want x", c.step, got)
		}
	}

	a := args("plain")
	wantSession := "spore-role/demo-project/feat/plain"
	if got := flagValue(a, "-s"); got != wantSession {
		t.Errorf("session = %q, want %q", got, wantSession)
	}
	if got := envValue(a, "SPORE_REVIEWER_INSTANCE"); got != "A" {
		t.Errorf("SPORE_REVIEWER_INSTANCE = %q, want the label A", got)
	}
	if got := envValue(args("effort"), "SPORE_REVIEWER_INSTANCE"); got != "" {
		t.Errorf("unlabelled step: SPORE_REVIEWER_INSTANCE = %q, want unset", got)
	}
	if got, want := envValue(a, "SPORE_STEP_OUTPUT"), filepath.Join(task.RoleTaskDir(root, "feat"), "p", "round-{round}.json"); got != want {
		t.Errorf("SPORE_STEP_OUTPUT = %q, want %q", got, want)
	}
	if got := envValue(a, "SPORE_STEP_SKILLS"); got != "one,two" {
		t.Errorf("SPORE_STEP_SKILLS = %q, want one,two", got)
	}
}

func TestClassifyLoop(t *testing.T) {
	root := t.TempDir()
	copyLoopFixture(t, "broken-next.toml", filepath.Join(root, "bootstrap", "loops", "broken.toml"))
	cases := []struct {
		loop, want string
		unknown    bool
		errSub     string
	}{
		{loop: "", want: ""},
		{loop: "role", want: "dev"},
		{loop: "dev", want: "dev"},
		{loop: "example", want: "example"},
		{loop: "roel", unknown: true},
		{loop: "broken", errSub: `unknown step "nowhere"`},
	}
	for _, c := range cases {
		got, err := classifyLoop(root, frontmatter.Meta{Slug: "s", Loop: c.loop})
		switch {
		case c.unknown:
			if !errors.Is(err, ErrUnknownLoop) {
				t.Errorf("loop %q: err = %v, want ErrUnknownLoop", c.loop, err)
			}
		case c.errSub != "":
			if err == nil || errors.Is(err, ErrUnknownLoop) || !strings.Contains(err.Error(), c.errSub) {
				t.Errorf("loop %q: err = %v, want a non-unknown error containing %q", c.loop, err, c.errSub)
			}
		default:
			if err != nil || got != c.want {
				t.Errorf("loop %q: = %q, %v, want %q", c.loop, got, err, c.want)
			}
		}
	}
}

// `loop: role` written to a task file reaches the dev loop through the
// frontmatter parser, classifyLoop and LoadLoop.
func TestLoopRoleFrontmatterResolvesToDev(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTaskWithExtra(t, tasksDir, "r", "active", "loop", "role")
	name, err := task.RequireLoop(tasksDir, "r")
	if err != nil || name != "role" {
		t.Fatalf("RequireLoop = %q, %v, want role", name, err)
	}
	raw, err := os.ReadFile(filepath.Join(tasksDir, "r.md"))
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := frontmatter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	loop, err := classifyLoop(root, m)
	if err != nil || loop != "dev" {
		t.Fatalf("classifyLoop = %q, %v, want dev", loop, err)
	}
	def, err := LoadLoop(root, loop)
	if err != nil || def.Name != "dev" || len(def.Steps) != 3 {
		t.Errorf("LoadLoop(%q) = %s with %d steps, %v; want the dev loop", loop, def.Name, len(def.Steps), err)
	}

	writeTask(t, tasksDir, "g", "active")
	if _, err := task.RequireLoop(tasksDir, "g"); err == nil {
		t.Errorf("RequireLoop accepted a task without loop:")
	}
}

func TestReconcileDrivesAnyLoadableLoop(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	project := filepath.Base(dirs.project)
	t.Cleanup(func() { killLoopTestSessions(project) })
	panes := installLoopPanes(t)
	writeTaskWithExtra(t, dirs.tasks, "ex", "active", "loop", "example")

	r, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(r.Spawned) != 0 || len(r.Warnings) != 0 {
		t.Errorf("Spawned = %v, Warnings = %q; want neither for a task on the bundled example loop", r.Spawned, r.Warnings)
	}
	want := []string{"spore-role/" + project + "/ex/draft"}
	if !equalSlices(panes.spawns, want) {
		t.Errorf("role spawns = %v, want %v", panes.spawns, want)
	}
}
