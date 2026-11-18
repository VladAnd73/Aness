package fleet

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// Acceptance scenarios for configurable loops
// (docs/todo/configurable-loops.md). Scenario 2 is roleloop_test.go.

// loopPaneFake stands in for tmux behind the role-pane seams. Wakes
// follow wakeSession's contract: one delivery per (session, marker
// stem, pane incarnation).
type loopPaneFake struct {
	panes      map[string]*paneProbe
	gen        map[string]int
	delivered  map[string]bool
	spawns     []string
	notes      map[string][]string
	kills      []string
	wakes      []loopWake
	now        time.Time
	afterSpawn func(f *loopPaneFake, session string)
}

type loopWake struct {
	session, msg string
}

func installLoopPanes(t *testing.T) *loopPaneFake {
	t.Helper()
	f := &loopPaneFake{
		panes:     map[string]*paneProbe{},
		gen:       map[string]int{},
		delivered: map[string]bool{},
		notes:     map[string][]string{},
		now:       time.Unix(1_800_000_000, 0),
	}
	prevSpawn, prevProbe, prevKill, prevNow, prevWake := spawnRole, probeRolePane, killRoleSession, nowFunc, wakeRole
	spawnRole = func(spec RoleSpawnSpec) (string, bool, error) {
		s, err := spec.SessionName()
		if err != nil {
			return "", false, err
		}
		if _, ok := f.panes[s]; ok {
			return s, false, nil
		}
		f.spawns = append(f.spawns, s)
		f.notes[s] = append(f.notes[s], spec.Note)
		f.gen[s]++
		f.panes[s] = &paneProbe{Exists: true, Command: "claude", Created: f.now, Content: "booting"}
		if f.afterSpawn != nil {
			f.afterSpawn(f, s)
		}
		return s, true, nil
	}
	probeRolePane = func(s string) (paneProbe, error) {
		if p, ok := f.panes[s]; ok {
			return *p, nil
		}
		return paneProbe{}, nil
	}
	killRoleSession = func(s string) {
		f.kills = append(f.kills, s)
		delete(f.panes, s)
	}
	nowFunc = func() time.Time { return f.now }
	wakeRole = func(spec RoleSpawnSpec, stem, msg string) (wakeOutcome, error) {
		s, err := spec.SessionName()
		if err != nil {
			return wakePending, err
		}
		if _, ok := f.panes[s]; !ok {
			return wakeNoSession, nil
		}
		key := fmt.Sprintf("%s\x00%s\x00%d", s, stem, f.gen[s])
		if !f.delivered[key] {
			f.delivered[key] = true
			f.wakes = append(f.wakes, loopWake{s, msg})
		}
		return wakeDelivered, nil
	}
	t.Cleanup(func() {
		spawnRole, probeRolePane, killRoleSession, nowFunc, wakeRole = prevSpawn, prevProbe, prevKill, prevNow, prevWake
	})
	return f
}

func (f *loopPaneFake) wakesTo(session string) []string {
	var out []string
	for _, w := range f.wakes {
		if w.session == session {
			out = append(out, w.msg)
		}
	}
	return out
}

// loopFixture is a git project with one active task on a loop copied
// from testdata/loops into the project's bootstrap/loops/.
type loopFixture struct {
	root, tasks, slug, project, inbox string
	panes                             *loopPaneFake
}

func newLoopFixture(t *testing.T, loopName, fixture string, roles ...string) *loopFixture {
	t.Helper()
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	coordState := t.TempDir()
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", coordState)
	project, err := task.ProjectName(dirs.project)
	if err != nil {
		t.Fatal(err)
	}
	f := &loopFixture{
		root:    dirs.project,
		tasks:   dirs.tasks,
		slug:    "demo",
		project: project,
		inbox:   filepath.Join(coordState, project, "inbox"),
	}
	copyLoopFixture(t, fixture, filepath.Join(f.root, "bootstrap", "loops", loopName+".toml"))
	writeLoopRoleBodies(t, f.root, roles...)
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", loopName)
	f.panes = installLoopPanes(t)
	return f
}

func copyLoopFixture(t *testing.T, fixture, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "loops", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLoopRoleBodies(t *testing.T, root string, roles ...string) {
	t.Helper()
	dir := filepath.Join(root, "bootstrap", "roles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		body := "# " + r + "\n\nStand-in role body for the loop acceptance tests.\n"
		if err := os.WriteFile(filepath.Join(dir, r+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *loopFixture) session(name string) string {
	return "spore-role/" + f.project + "/" + f.slug + "/" + name
}

func (f *loopFixture) tick(t *testing.T) LoopState {
	t.Helper()
	st, err := DriveLoop(f.root, f.tasks, f.slug)
	if err != nil {
		t.Fatalf("DriveLoop: %v", err)
	}
	return st
}

// loopTaskDir is the per-task file tree the design fixes at
// <projectRoot>/.spore/<slug>/.
func loopTaskDir(root, slug string) string {
	return filepath.Join(root, ".spore", slug)
}

func (f *loopFixture) path(rel string) string {
	return filepath.Join(loopTaskDir(f.root, f.slug), rel)
}

func (f *loopFixture) write(t *testing.T, rel, body string) {
	t.Helper()
	p := f.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *loopFixture) exists(rel string) bool {
	_, err := os.Stat(f.path(rel))
	return err == nil
}

func (f *loopFixture) stateMarkers(prefix string) []string {
	entries, _ := os.ReadDir(f.path("state"))
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	return out
}

// inboxBodies reads every tell envelope under the coordinator inbox,
// including ones a watcher already moved to .pending/ or read/.
func inboxBodies(t *testing.T, inbox string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(inbox, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") || strings.Contains(p, "/.tmp/") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var env struct {
			Body string `json:"body"`
		}
		if json.Unmarshal(b, &env) == nil {
			out = append(out, env.Body)
		}
		return nil
	})
	return out
}

func countContaining(list []string, sub string) int {
	n := 0
	for _, s := range list {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

func fileLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func verdictJSON(author, verdict string) string {
	return fmt.Sprintf(`{"author":%q,"verdict":%q,"summary":"test verdict","comments":[]}`, author, verdict)
}

func loopTmuxSessions(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", testTmuxSocket, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func killLoopTestSessions(project string) {
	out, err := exec.Command("tmux", "-L", testTmuxSocket, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "spore/"+project+"/") || strings.HasPrefix(line, "spore-role/"+project+"/") {
			_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", line).Run()
		}
	}
}

func sessionsWithPrefix(sessions []string, prefix string) []string {
	var out []string
	for _, s := range sessions {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func TestAcceptanceS1NoLoopTaskGetsSingleWorker(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	project := filepath.Base(dirs.project)
	t.Cleanup(func() { killLoopTestSessions(project) })
	panes := installLoopPanes(t)

	writeTask(t, dirs.tasks, "plain", "active")
	writeTaskWithExtra(t, dirs.tasks, "keyed", "active", "loop", "role")

	r, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3})
	if err != nil {
		t.Logf("Reconcile: %v", err)
	}
	if want := []string{"plain"}; !equalSlices(r.Spawned, want) {
		t.Errorf("Spawned = %v, want %v (a loop task must not get a generic worker)", r.Spawned, want)
	}
	sessions := loopTmuxSessions(t)
	if got := sessionsWithPrefix(sessions, "spore/"+project+"/plain"); len(got) != 1 {
		t.Errorf("worker sessions for plain = %v, want exactly one", got)
	}
	if got := sessionsWithPrefix(sessions, "spore/"+project+"/keyed"); len(got) != 0 {
		t.Errorf("loop task keyed got a generic worker session: %v", got)
	}
	rolePrefix := "spore-role/" + project + "/plain/"
	if got := sessionsWithPrefix(append(sessions, panes.spawns...), rolePrefix); len(got) != 0 {
		t.Errorf("task without loop got role panes: %v", got)
	}
	if _, err := os.Stat(loopTaskDir(dirs.project, "plain")); err == nil {
		t.Errorf("task without loop got a %s dir", loopTaskDir(dirs.project, "plain"))
	}
}

// devStep writes one upstream role file (raw JSON, upstream's paths and
// shapes) and optionally checks the snapshot the dev loop derives.
type devStep struct {
	who     string // "eng", "A" or "B"
	round   int
	verdict string
	want    *Snapshot
}

func writeDevStep(t *testing.T, root, slug string, s devStep) {
	t.Helper()
	dir := loopTaskDir(root, slug)
	var p, body string
	if s.who == "eng" {
		p = filepath.Join(dir, "responses", fmt.Sprintf("engineer-round-%d.json", s.round))
		body = `{"author":"engineer","addressed":["placeholder"],"pushback":[]}`
	} else {
		p = filepath.Join(dir, "reviews", s.who, fmt.Sprintf("round-%d.json", s.round))
		body = verdictJSON(s.who, s.verdict)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceS3DevLoopMatchesRoleLoop(t *testing.T) {
	const (
		ok = task.VerdictApprove
		rc = task.VerdictRequestChanges
		A  = task.ReviewerA
		B  = task.ReviewerB
	)
	snap := func(p Phase, eng int, rev task.ReviewerInstance, round int, last string) *Snapshot {
		s := &Snapshot{Phase: p, EngineerRound: eng, CurrentReviewer: rev, ReviewerRound: round, LastVerdict: last}
		if rev != "" {
			s.MaxRounds = MaxReviewerRounds
		}
		return s
	}
	cases := []struct {
		name    string
		initial *Snapshot
		steps   []devStep
	}{
		{"happy path", snap(PhaseEngineerInitial, 0, "", 0, ""), []devStep{
			{"eng", 1, "", snap(PhaseReviewA, 1, A, 0, "")},
			{"A", 1, ok, snap(PhaseReviewB, 1, B, 0, "")},
			{"B", 1, ok, snap(PhaseDone, 1, B, 1, ok)},
		}},
		{"revise loop", nil, []devStep{
			{"eng", 1, "", nil},
			{"A", 1, rc, snap(PhaseEngineerReviseA, 1, A, 1, rc)},
			{"eng", 2, "", snap(PhaseReviewA, 2, A, 1, rc)},
			{"A", 2, ok, snap(PhaseReviewB, 2, B, 0, "")},
			{"B", 1, rc, snap(PhaseEngineerReviseB, 2, B, 1, rc)},
			{"eng", 3, "", snap(PhaseReviewB, 3, B, 1, rc)},
			{"B", 2, ok, snap(PhaseDone, 3, B, 2, ok)},
		}},
		{"escalate at round 3 on A", nil, []devStep{
			{"eng", 1, "", nil}, {"A", 1, rc, nil},
			{"eng", 2, "", nil}, {"A", 2, rc, nil},
			{"eng", 3, "", nil},
			{"A", 3, rc, snap(PhaseEscalated, 3, A, 3, rc)},
			{"eng", 4, "", snap(PhaseEscalated, 4, A, 3, rc)},
		}},
		{"escalate at round 3 on B", nil, []devStep{
			{"eng", 1, "", nil}, {"A", 1, ok, nil},
			{"B", 1, rc, nil}, {"eng", 2, "", nil},
			{"B", 2, rc, nil}, {"eng", 3, "", nil},
			{"B", 3, rc, snap(PhaseEscalated, 3, B, 3, rc)},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			slug := "dev"
			def, err := LoadLoop(root, "dev")
			if err != nil {
				t.Fatalf("LoadLoop dev: %v", err)
			}
			if len(def.Steps) != 3 {
				t.Errorf("dev loop has %d steps, want 3 (engineer, reviewer A, reviewer B)", len(def.Steps))
			}
			check := func(label string, want *Snapshot) {
				t.Helper()
				if want == nil {
					return
				}
				got, err := DeriveLoopSnapshot(root, slug, def)
				if err != nil {
					t.Fatalf("%s: DeriveLoopSnapshot: %v", label, err)
				}
				got.Slug = ""
				if got != *want {
					t.Errorf("%s: dev loop snapshot differs from role loop\n want: %+v\n got:  %+v", label, *want, got)
				}
			}
			check("initial", c.initial)
			for _, s := range c.steps {
				writeDevStep(t, root, slug, s)
				check(fmt.Sprintf("after %s round %d %s", s.who, s.round, s.verdict), s.want)
			}
		})
	}
}

func TestAcceptanceS4ThreeStepLoopAdvancesInOrder(t *testing.T) {
	f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
	sa, sb := f.session("a"), f.session("b")

	f.tick(t)
	if want := []string{sa}; !equalSlices(f.panes.spawns, want) {
		t.Fatalf("tick 1: spawns = %v, want %v (step a starts first)", f.panes.spawns, want)
	}
	f.tick(t)
	if want := []string{sa}; !equalSlices(f.panes.spawns, want) {
		t.Fatalf("tick 2, a's output missing: spawns = %v, want %v (b must wait for a)", f.panes.spawns, want)
	}

	f.write(t, "a/round-1.json", `{"result":"x"}`)
	f.tick(t)
	if want := []string{sa, sb}; !equalSlices(f.panes.spawns, want) {
		t.Fatalf("tick 3, a done: spawns = %v, want %v", f.panes.spawns, want)
	}
	if countContaining(f.panes.kills, sa) != 1 {
		t.Errorf("tick 3: kills = %v, want a's session killed once b starts", f.panes.kills)
	}

	f.write(t, "b/round-1.json", `{"result":"y"}`)
	st := f.tick(t)
	if st.Terminal != "done" {
		t.Errorf("tick 4, b done and c is an action: state = %+v, want Terminal done", st)
	}
	if n := countContaining(inboxBodies(t, f.inbox), "trio reached c"); n != 1 {
		t.Errorf("tick 4: notify-coordinator action ran %d times, want 1", n)
	}
	f.tick(t)
	if n := countContaining(inboxBodies(t, f.inbox), "trio reached c"); n != 1 {
		t.Errorf("tick 5: notify-coordinator action ran %d times in total, want still 1", n)
	}
	if !f.exists("state/ready") {
		t.Errorf("done loop has no state/ready marker")
	}
}

func TestAcceptanceS5FailingCommandBlocksAndNotifiesOnce(t *testing.T) {
	f := newLoopFixture(t, "checked", "checked.toml", "alpha")
	count := filepath.Join(t.TempDir(), "count")
	t.Setenv("COUNT_FILE", count)
	sa := f.session("a")

	f.tick(t)
	f.write(t, "a/round-1.json", `{"result":"x"}`)
	for i := 1; i <= 3; i++ {
		if st := f.tick(t); st.Step != "a" {
			t.Errorf("tick %d with a failing check: state = %+v, want Step a", i, st)
		}
	}
	if n := fileLines(t, count); n != 1 {
		t.Errorf("command ran %d times over three ticks on unchanged files, want 1", n)
	}
	bodies := inboxBodies(t, f.inbox)
	if n := countContaining(bodies, "check failed"); n != 1 {
		t.Errorf("coordinator got %d check-failed notices, want 1; inbox = %q", n, bodies)
	} else if countContaining(bodies, "boom") != 1 {
		t.Errorf("check-failed notice lacks the command output %q; inbox = %q", "boom", bodies)
	}
	wakes := f.panes.wakesTo(sa)
	if len(wakes) != 1 || !strings.Contains(wakes[0], "boom") {
		t.Errorf("wakes to a = %q, want exactly one carrying the command output %q", wakes, "boom")
	}
	if m := f.stateMarkers("escalated-"); len(m) != 0 {
		t.Errorf("a failing check escalated (%v); it must not count toward max_rounds", m)
	}

	if want := "Write round 2 now: " + f.path("a/round-2.json"); len(wakes) == 1 && !strings.Contains(wakes[0], want) {
		t.Errorf("check-failure wake lacks %q:\n%s", want, wakes[0])
	}

	f.write(t, "a/round-1.json", `{"result":"changed"}`)
	f.tick(t)
	if n := fileLines(t, count); n != 2 {
		t.Errorf("after a's file changed the command ran %d times in total, want 2", n)
	}

	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_FILE", pass)
	f.write(t, "a/round-2.json", `{"result":"fixed","verdict":"again"}`)
	st := f.tick(t)
	if st.Blocked != "" || st.Terminal != "" {
		t.Errorf("round 2 passes its check and asks for another round: state = %+v, want a running round 3 (the blocked round 1 does not count toward max_rounds 2)", st)
	}
	if m := f.stateMarkers("escalated-"); len(m) != 0 {
		t.Errorf("escalated (%v) at the first passing round: the failed round counted toward max_rounds", m)
	}
}

func TestAcceptanceS6RequestChangesReturnsThenEscalatesAtCap(t *testing.T) {
	f := newLoopFixture(t, "revise", "revise.toml", "author", "reviewer")
	sAuthor, sReview := f.session("author"), f.session("review")

	f.write(t, "author/round-1.json", `{"notes":"first"}`)
	f.write(t, "review/round-1.json", verdictJSON("review", task.VerdictRequestChanges))
	st := f.tick(t)
	if st.Step != "author" || st.Phase != "author-revise-review" {
		t.Errorf("after review round 1 request_changes: state = %+v, want Step author, Phase author-revise-review", st)
	}
	verdictPath := filepath.Join(".spore", f.slug, "review", "round-1.json")
	if w := f.panes.wakesTo(sAuthor); countContaining(w, verdictPath) != 1 {
		t.Errorf("wakes to author = %q, want one naming %s", w, verdictPath)
	}

	f.write(t, "author/round-2.json", `{"notes":"second"}`)
	st = f.tick(t)
	if st.Step != "review" {
		t.Errorf("after author round 2: state = %+v, want Step review (return to the sender)", st)
	}
	respPath := filepath.Join("author", "round-2.json")
	if w := f.panes.wakesTo(sReview); countContaining(w, respPath) != 1 {
		t.Errorf("wakes to review = %q, want one naming %s", w, respPath)
	}

	f.write(t, "review/round-2.json", verdictJSON("review", task.VerdictRequestChanges))
	st = f.tick(t)
	if st.Terminal != "escalated" {
		t.Errorf("review round 2 request_changes at max_rounds 2: state = %+v, want Terminal escalated", st)
	}
	if !f.exists("state/escalated-review-2") {
		t.Errorf("no state/escalated-review-2 marker; markers = %v", f.stateMarkers(""))
	}
	if n := countContaining(inboxBodies(t, f.inbox), "escalated"); n != 1 {
		t.Errorf("coordinator got %d escalation notices, want 1", n)
	}
}

func TestAcceptanceS7ProjectOverrideWins(t *testing.T) {
	root := t.TempDir()
	stepNames := func(def LoopDef) []string {
		var out []string
		for _, s := range def.Steps {
			out = append(out, s.Name)
		}
		return out
	}

	def, err := LoadLoop(root, "example")
	if err != nil {
		t.Fatalf("LoadLoop example (embedded): %v", err)
	}
	embedded := stepNames(def)
	if len(embedded) == 0 {
		t.Errorf("embedded example loop has no steps")
	}
	if countContaining(embedded, "override-only") != 0 {
		t.Errorf("embedded example loop already has step override-only: %v", embedded)
	}

	writeLoopRoleBodies(t, root, "alpha")
	override := `name = "example"

[[steps]]
name = "override-only"
kind = "agent"
role = "alpha"
session = "o"
output = "o/round-{round}.json"
shape = "json"
required = ["notes"]
`
	p := filepath.Join(root, "bootstrap", "loops", "example.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	def, err = LoadLoop(root, "example")
	if err != nil {
		t.Fatalf("LoadLoop example (override): %v", err)
	}
	if got, want := stepNames(def), []string{"override-only"}; !equalSlices(got, want) {
		t.Errorf("with a project override, steps = %v, want %v", got, want)
	}
}

func TestAcceptanceS8InvalidLoopRefusesAndSpawnsNothing(t *testing.T) {
	requireToolchain(t)
	cases := []struct{ name, fixture, wantErr string }{
		{"unknown next", "broken-next.toml", `unknown step "nowhere"`},
		{"unknown kind", "broken-kind.toml", `unknown kind "robot"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dirs := newTestDirs(t)
			gitInit(t, dirs.project)
			mustEnable(t)
			t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
			t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
			project := filepath.Base(dirs.project)
			t.Cleanup(func() { killLoopTestSessions(project) })
			panes := installLoopPanes(t)
			override := filepath.Join(dirs.project, "bootstrap", "loops", "broken.toml")
			copyLoopFixture(t, c.fixture, override)
			writeLoopRoleBodies(t, dirs.project, "alpha")

			_, err := LoadLoop(dirs.project, "broken")
			if err == nil {
				t.Errorf("LoadLoop accepted %s", c.fixture)
			} else if !strings.Contains(err.Error(), c.wantErr) || !strings.Contains(err.Error(), override) {
				t.Errorf("LoadLoop error = %q, want it to contain %q and the file path %s", err, c.wantErr, override)
			}

			writeTaskWithExtra(t, dirs.tasks, "bad", "active", "loop", "broken")
			r, rerr := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3})
			if rerr != nil {
				t.Logf("Reconcile: %v", rerr)
			}
			if len(r.Spawned) != 0 {
				t.Errorf("Spawned = %v for a task on an invalid loop, want none", r.Spawned)
			}
			if len(panes.spawns) != 0 {
				t.Errorf("role panes spawned for a task on an invalid loop: %v", panes.spawns)
			}
			sessions := loopTmuxSessions(t)
			if got := append(sessionsWithPrefix(sessions, "spore/"+project+"/bad"),
				sessionsWithPrefix(sessions, "spore-role/"+project+"/bad/")...); len(got) != 0 {
				t.Errorf("sessions started for a task on an invalid loop: %v", got)
			}
			var hits []string
			for _, w := range r.Warnings {
				if strings.Contains(w, "bad") && strings.Contains(w, c.wantErr) {
					hits = append(hits, w)
				}
			}
			if len(hits) != 1 {
				t.Errorf("Warnings = %q, want exactly one naming task bad and %q", r.Warnings, c.wantErr)
			}
		})
	}
}

// spawnRecordKeys returns the keys of every spawn-record file in the
// project's state dir (fleet-spawns.json today).
func spawnRecordKeys(t *testing.T, root string) []string {
	t.Helper()
	dir, err := task.StateDirForProject(root)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*spawns*.json"))
	var keys []string
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) == nil {
			for k := range m {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func TestAcceptanceS9DyingPaneBackoffAndCap(t *testing.T) {
	t.Run("session vanishes after spawn", func(t *testing.T) {
		f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
		f.panes.afterSpawn = func(p *loopPaneFake, s string) { delete(p.panes, s) }

		for i := 1; i <= 3; i++ {
			f.tick(t)
		}
		if n := len(f.panes.spawns); n != 1 {
			t.Errorf("three ticks at one instant spawned %d times, want 1 (spawn backoff)", n)
		}
		f.panes.now = f.panes.now.Add(61 * time.Second)
		f.tick(t)
		if n := len(f.panes.spawns); n != 2 {
			t.Errorf("after the 1m backoff spawns = %d, want 2", n)
		}
		for i := 1; i <= 3; i++ {
			f.tick(t)
		}
		if n := len(f.panes.spawns); n != 2 {
			t.Errorf("three more ticks at one instant: spawns = %d, want still 2 (backoff doubled)", n)
		}
		var hits []string
		for _, k := range spawnRecordKeys(t, f.root) {
			if strings.Contains(k, f.slug) && k != f.slug {
				hits = append(hits, k)
			}
		}
		if len(hits) == 0 {
			t.Errorf("spawn backoff file has no record for loop pane a; keys = %v", spawnRecordKeys(t, f.root))
		}
	})

	t.Run("pane dies after every spawn", func(t *testing.T) {
		f := newLoopFixture(t, "trio", "trio.toml", "alpha", "beta")
		f.panes.afterSpawn = func(p *loopPaneFake, s string) { p.panes[s].Dead = true }

		f.tick(t)
		for i := 1; i <= MaxRoleRespawns; i++ {
			f.panes.now = f.panes.now.Add(31 * time.Minute)
			f.tick(t)
		}
		if n, want := len(f.panes.spawns), 1+MaxRoleRespawns; n != want {
			t.Errorf("spawns = %d after %d deaths, want %d (first spawn plus capped respawns)", n, MaxRoleRespawns, want)
		}
		f.panes.now = f.panes.now.Add(31 * time.Minute)
		f.tick(t)
		if n, want := len(f.panes.spawns), 1+MaxRoleRespawns; n != want {
			t.Errorf("spawns = %d after the cap, want still %d", n, want)
		}
		if n := countContaining(inboxBodies(t, f.inbox), "died again after 3 respawns"); n != 1 {
			t.Errorf("coordinator got %d respawn-cap notices, want 1; inbox = %q", n, inboxBodies(t, f.inbox))
		}
	})
}
