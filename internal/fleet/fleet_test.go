package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/matter"
	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

func TestReconcileShortCircuitsWhenDisabled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dirs := newTestDirs(t)
	writeTask(t, dirs.tasks, "alpha", "active")

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !r.Disabled {
		t.Errorf("expected Disabled=true with no flag set")
	}
	if len(r.Spawned)+len(r.Reaped)+len(r.Kept)+len(r.Skipped) != 0 {
		t.Errorf("expected empty actions when disabled, got %+v", r)
	}
}

func TestReconcileSpawnsAndReaps(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	writeTask(t, dirs.tasks, "alpha", "active")
	writeTask(t, dirs.tasks, "beta", "active")
	writeTask(t, dirs.tasks, "gamma", "draft")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"alpha", "beta"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v", got, want)
	}
	if len(r.Reaped) != 0 || len(r.Kept) != 0 || len(r.Skipped) != 0 {
		t.Errorf("first pass should not reap/keep/skip, got %+v", r)
	}

	r2, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile pass 2: %v", err)
	}
	if got, want := r2.Kept, []string{"alpha", "beta"}; !equalSlices(got, want) {
		t.Errorf("Kept (pass 2) = %v, want %v", got, want)
	}
	if len(r2.Spawned) != 0 || len(r2.Reaped) != 0 {
		t.Errorf("pass 2 should be a no-op, got %+v", r2)
	}

	flipStatus(t, dirs.tasks, "alpha", "done")

	r3, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile pass 3: %v", err)
	}
	if got, want := r3.Reaped, []string{"alpha"}; !equalSlices(got, want) {
		t.Errorf("Reaped (pass 3) = %v, want %v", got, want)
	}
	if got, want := r3.Kept, []string{"beta"}; !equalSlices(got, want) {
		t.Errorf("Kept (pass 3) = %v, want %v", got, want)
	}

	// Pause beta: reconcile must keep the session alive (pause is
	// the operator-attached state, not a teardown signal).
	flipStatus(t, dirs.tasks, "beta", "paused")

	r4, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile pass 4: %v", err)
	}
	if len(r4.Reaped) != 0 {
		t.Errorf("Reaped (pass 4 / paused) = %v, want []", r4.Reaped)
	}
	if got, want := r4.Kept, []string{"beta"}; !equalSlices(got, want) {
		t.Errorf("Kept (pass 4 / paused) = %v, want %v", got, want)
	}
}

func TestReconcileRespectsMaxWorkers(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	for _, slug := range []string{"a", "b", "c", "d", "e"} {
		writeTask(t, dirs.tasks, slug, "active")
	}
	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  2,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"a", "b"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v", got, want)
	}
	if got, want := r.Skipped, []string{"c", "d", "e"}; !equalSlices(got, want) {
		t.Errorf("Skipped = %v, want %v", got, want)
	}
}

func TestReconcileSpawnsByMatterSortOrder(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	// Slug order is alpha < bravo < charlie < delta. matter_sort_order
	// inverts it: charlie sits at the top of the kanban column. With
	// MaxWorkers=2 the cap should pick charlie + bravo, not the
	// alphabetical alpha + bravo.
	writeTaskWithExtra(t, dirs.tasks, "alpha", "active", matter.MatterSortOrderKey, "30")
	writeTaskWithExtra(t, dirs.tasks, "bravo", "active", matter.MatterSortOrderKey, "20")
	writeTaskWithExtra(t, dirs.tasks, "charlie", "active", matter.MatterSortOrderKey, "10")
	writeTaskWithExtra(t, dirs.tasks, "delta", "active", matter.MatterSortOrderKey, "40")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  2,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"bravo", "charlie"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v (top two by matter_sort_order)", got, want)
	}
	if got, want := r.Skipped, []string{"alpha", "delta"}; !equalSlices(got, want) {
		t.Errorf("Skipped = %v, want %v", got, want)
	}
}

func TestReconcileSortsStampedBeforeUnstamped(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	// "zulu" carries a stamp, "alpha" does not. Stamped tasks sort
	// before unstamped so a matter-managed ticket beats an ad-hoc
	// task when slots are scarce.
	writeTaskWithExtra(t, dirs.tasks, "zulu", "active", matter.MatterSortOrderKey, "5")
	writeTask(t, dirs.tasks, "alpha", "active")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  1,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"zulu"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v", got, want)
	}
	if got, want := r.Skipped, []string{"alpha"}; !equalSlices(got, want) {
		t.Errorf("Skipped = %v, want %v", got, want)
	}
}

func TestEnableDisableFlag(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	on, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if on {
		t.Error("expected disabled with fresh state dir")
	}
	if err := Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	on, err = Enabled()
	if err != nil || !on {
		t.Errorf("after Enable: enabled=%v err=%v, want true nil", on, err)
	}
	// Idempotent.
	if err := Enable(); err != nil {
		t.Errorf("Enable second call: %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	on, err = Enabled()
	if err != nil || on {
		t.Errorf("after Disable: enabled=%v err=%v, want false nil", on, err)
	}
	if err := Disable(); err != nil {
		t.Errorf("Disable on missing flag: %v", err)
	}
}

func TestReconcileAssignsAgentFromMix(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	if err := os.WriteFile(filepath.Join(dirs.project, "spore.toml"), []byte(`
[fleet.workers]
default = "claude"

[fleet.workers.ratio]
claude = 70
codex = 30

[fleet.workers.rules]
mechanical = "codex"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// alpha/beta/gamma have no agent set; the rule pins delta to codex
	// regardless of ratio; epsilon already has agent: claude pinned and
	// must not be rewritten.
	writeTask(t, dirs.tasks, "alpha", "active")
	writeTask(t, dirs.tasks, "beta", "active")
	writeTask(t, dirs.tasks, "gamma", "active")
	writeTaskWithExtra(t, dirs.tasks, "delta", "active", "complexity", "mechanical")
	writeTaskWithAgent(t, dirs.tasks, "epsilon", "active", "claude")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  10,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"alpha", "beta", "delta", "epsilon", "gamma"}; !equalSlices(got, want) {
		t.Fatalf("Spawned = %v, want %v", got, want)
	}

	gotAgents := map[string]string{}
	for _, slug := range r.Spawned {
		raw, err := os.ReadFile(filepath.Join(dirs.tasks, slug+".md"))
		if err != nil {
			t.Fatal(err)
		}
		m, _, err := frontmatter.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		gotAgents[slug] = m.Agent
	}

	// Rule pin: complexity=mechanical -> codex.
	if gotAgents["delta"] != "codex" {
		t.Errorf("delta agent = %q, want codex (rule)", gotAgents["delta"])
	}
	// Explicit pin survives.
	if gotAgents["epsilon"] != "claude" {
		t.Errorf("epsilon agent = %q, want claude (explicit)", gotAgents["epsilon"])
	}
	// Among alpha/beta/gamma (3 spawns under 70/30 with delta=codex
	// already counted as the running codex), the balancer picks claude
	// for two and may pick codex for the third. Each must be a known
	// agent and the totals must respect the target split: at least 2
	// claude across the unrestricted three is our floor.
	claudeAmongFree := 0
	for _, slug := range []string{"alpha", "beta", "gamma"} {
		switch a := gotAgents[slug]; a {
		case "claude":
			claudeAmongFree++
		case "codex":
		default:
			t.Errorf("%s agent = %q, want claude or codex", slug, a)
		}
	}
	if claudeAmongFree < 2 {
		t.Errorf("free spawns leaned claude=%d (want >= 2 from 70/30 with one codex already from rule)", claudeAmongFree)
	}
}

func TestLoadMaxWorkersTOML(t *testing.T) {
	root := t.TempDir()
	if got, err := LoadMaxWorkers(root); err != nil || got != DefaultMaxWorkers {
		t.Fatalf("missing toml: got %d err %v, want %d nil", got, err, DefaultMaxWorkers)
	}
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte("[fleet]\nmax_workers = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMaxWorkers(root)
	if err != nil {
		t.Fatalf("LoadMaxWorkers: %v", err)
	}
	if got != 7 {
		t.Errorf("max_workers = %d, want 7", got)
	}
}

type testDirs struct {
	project string
	tasks   string
}

func newTestDirs(t *testing.T) testDirs {
	t.Helper()
	// Use the test name as the project basename so session names
	// (`spore/<basename>/<slug>`) don't collide with sibling tests:
	// t.TempDir() produces unique full paths but reuses basename
	// `001` across the binary's tests.
	name := strings.ReplaceAll(t.Name(), "/", "_")
	project := filepath.Join(t.TempDir(), name)
	tasks := filepath.Join(project, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	return testDirs{project: project, tasks: tasks}
}

func gitInit(t *testing.T, repo string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func mustEnable(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
}

func writeTask(t *testing.T, tasksDir, slug, status string) {
	t.Helper()
	m := frontmatter.Meta{Status: status, Slug: slug, Title: slug}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskWithAgent(t *testing.T, tasksDir, slug, status, agent string) {
	t.Helper()
	m := frontmatter.Meta{Status: status, Slug: slug, Title: slug, Agent: agent}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskWithExtra(t *testing.T, tasksDir, slug, status, key, val string) {
	t.Helper()
	m := frontmatter.Meta{
		Status: status,
		Slug:   slug,
		Title:  slug,
		Extra:  map[string]string{key: val},
	}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskWithExtras(t *testing.T, tasksDir, slug, status string, extras map[string]string) {
	t.Helper()
	m := frontmatter.Meta{
		Status: status,
		Slug:   slug,
		Title:  slug,
		Extra:  extras,
	}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func flipStatus(t *testing.T, tasksDir, slug, status string) {
	t.Helper()
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, body, err := frontmatter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.Status = status
	if err := os.WriteFile(path, frontmatter.Write(m, body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux not available: %v", err)
	}
}

func killSporeSessions(projectRoot string) {
	out, err := exec.Command("tmux", "-L", testTmuxSocket, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	prefix := "spore/" + filepath.Base(projectRoot) + "/"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, prefix) {
			_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", line).Run()
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeRoleLoopTask(t *testing.T, tasksDir, slug, status string) {
	t.Helper()
	m := frontmatter.Meta{Status: status, Slug: slug, Title: slug, Loop: task.LoopRole}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileRoleLoopOptIns covers the role-loop opt-in matrix in a
// single pass:
//
//   - "keyed": `loop: role` in frontmatter, no `.spore/<slug>/` dir.
//     This is the race window observed on the token-monitor-finish-unit
//     run: the watcher-triggered pass reacts to the status flip before
//     any dir is seeded. The key alone must keep the homogeneous spawn
//     away, and the drive pass must create the dir itself.
//   - "staged": no key, `.spore/<slug>/spec.md` staged by a
//     coordinator. Dir presence is not an opt-in: exactly one generic
//     worker and no role pane.
//   - "plain": neither signal. Generic worker path unchanged.
func TestReconcileRoleLoopOptIns(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	shimClaudeOnPath(t)

	writeRoleLoopTask(t, dirs.tasks, "keyed", "active")
	writeTask(t, dirs.tasks, "staged", "active")
	if err := task.WriteSpec(dirs.project, "staged", []byte("review brief\n")); err != nil {
		t.Fatal(err)
	}
	writeTask(t, dirs.tasks, "plain", "active")

	t.Cleanup(func() {
		killSporeSessions(dirs.project)
		killRoleSessions(dirs.project)
	})

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"plain", "staged"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v", got, want)
	}
	if len(r.Skipped) != 0 {
		t.Errorf("Skipped = %v, want []", r.Skipped)
	}

	project, err := task.ProjectName(dirs.project)
	if err != nil {
		t.Fatal(err)
	}
	if hasSession(fmt.Sprintf("spore/%s/keyed", project)) {
		t.Errorf("keyed: generic worker session spawned for a role-looped task")
	}
	if !hasSession(fmt.Sprintf("spore-role/%s/keyed/engineer", project)) {
		t.Errorf("keyed: engineer pane missing; drive pass did not proceed")
	}
	spec := filepath.Join(task.RoleTaskDir(dirs.project, "keyed"), "spec.md")
	if _, err := os.Stat(spec); err != nil {
		t.Errorf("keyed: spec cache missing at %s: %v", spec, err)
	}
	if !hasSession(fmt.Sprintf("spore/%s/staged", project)) {
		t.Errorf("staged: generic worker session missing")
	}
	if hasSession(fmt.Sprintf("spore-role/%s/staged/engineer", project)) {
		t.Errorf("staged: engineer pane spawned; .spore/<slug>/ presence must not opt into the role loop")
	}
}

// TestReconcileWarnsOnUnknownLoop pins that a typo'd `loop:` value
// keeps the generic worker fallback but surfaces a warning naming the
// slug and the bad value.
func TestReconcileWarnsOnUnknownLoop(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	m := frontmatter.Meta{Status: "active", Slug: "typo", Title: "typo", Loop: "roel"}
	if err := os.WriteFile(filepath.Join(dirs.tasks, "typo.md"), frontmatter.Write(m, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTask(t, dirs.tasks, "plain", "active")
	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir:    dirs.tasks,
		ProjectRoot: dirs.project,
		MaxWorkers:  3,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"plain", "typo"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v", got, want)
	}
	if len(r.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", r.Warnings)
	}
	if w := r.Warnings[0]; !strings.Contains(w, "typo") || !strings.Contains(w, `"roel"`) {
		t.Errorf("warning %q should name slug typo and value \"roel\"", w)
	}
}

// shimClaudeOnPath prepends a fake `claude` binary (exec sleep 30) to
// PATH. Role panes hardcode the claude binary (RoleSpawnSpec.Agent
// defaults to it and SPORE_AGENT_BINARY does not reach them), so
// without the shim a drive-spawned engineer pane would exec the real
// claude CLI on a dev host.
func shimClaudeOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func killRoleSessions(projectRoot string) {
	out, err := exec.Command("tmux", "-L", testTmuxSocket, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	prefix := "spore-role/" + filepath.Base(projectRoot) + "/"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, prefix) {
			_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", line).Run()
		}
	}
}

// TestReconcileRoleLoopedSkipsSpawnBackoff pins that the generic
// worker's respawn backoff never applies to a role-looped task: the
// loop driver owns its panes, so a stale quick-death record must not
// park it in BackedOff, and reconcile must not write a record for it.
func TestReconcileRoleLoopedSkipsSpawnBackoff(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	shimClaudeOnPath(t)

	writeRoleLoopTask(t, dirs.tasks, "keyed", "active")
	t.Cleanup(func() {
		killSporeSessions(dirs.project)
		killRoleSessions(dirs.project)
	})

	stateDir, err := task.StateDirForProject(dirs.project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := map[string]spawnRecord{"keyed": {Last: time.Now().Add(-10 * time.Second)}}
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	spawns := filepath.Join(stateDir, "fleet-spawns.json")
	if err := os.WriteFile(spawns, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	for pass := 1; pass <= 2; pass++ {
		r, err := Reconcile(Config{
			TasksDir:    dirs.tasks,
			ProjectRoot: dirs.project,
			MaxWorkers:  3,
		})
		if err != nil {
			t.Fatalf("pass %d: Reconcile: %v", pass, err)
		}
		if len(r.BackedOff) != 0 || len(r.Failed) != 0 {
			t.Errorf("pass %d: BackedOff = %v, Failed = %v, want both empty", pass, r.BackedOff, r.Failed)
		}
		recs := map[string]json.RawMessage{}
		if b, err := os.ReadFile(spawns); err == nil {
			if err := json.Unmarshal(b, &recs); err != nil {
				t.Fatalf("pass %d: parse %s: %v", pass, spawns, err)
			}
		}
		if _, ok := recs["keyed"]; ok {
			t.Errorf("pass %d: fleet-spawns.json has a record for role-looped keyed: %s", pass, recs["keyed"])
		}
	}
}

// TestReconcileDrivesLoopsPastAFailedSpawn: a generic task whose
// worker cannot start (a typo under [worker]) is recorded and the
// pass goes on to drive the looped task.
func TestReconcileDrivesLoopsPastAFailedSpawn(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	shimClaudeOnPath(t)
	if err := os.WriteFile(filepath.Join(dirs.project, "spore.toml"), []byte("[worker]\nisolate_netwrok = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTask(t, dirs.tasks, "plain", "active")
	writeRoleLoopTask(t, dirs.tasks, "keyed", "active")
	t.Cleanup(func() {
		killSporeSessions(dirs.project)
		killRoleSessions(dirs.project)
	})

	r, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3})
	if err == nil || !strings.Contains(err.Error(), "ensure plain") {
		t.Errorf("Reconcile err = %v, want the failed spawn of plain reported", err)
	}
	if !equalSlices(r.Failed, []string{"plain"}) {
		t.Errorf("Failed = %v, want [plain]", r.Failed)
	}
	spec := filepath.Join(task.RoleTaskDir(dirs.project, "keyed"), "spec.md")
	if _, statErr := os.Stat(spec); statErr != nil {
		t.Errorf("keyed: loop not driven after plain's spawn failed: %v", statErr)
	}
}

// TestReconcileDrivesLoopsPastAFailedAgentAssign: a generic task whose
// agent pick cannot be written back (a read-only task file) is
// recorded and the pass goes on to drive the looped task.
func TestReconcileDrivesLoopsPastAFailedAgentAssign(t *testing.T) {
	requireToolchain(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file")
	}

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	shimClaudeOnPath(t)
	writeTask(t, dirs.tasks, "plain", "active")
	plain := filepath.Join(dirs.tasks, "plain.md")
	if err := os.Chmod(plain, 0o444); err != nil {
		t.Fatal(err)
	}
	writeRoleLoopTask(t, dirs.tasks, "keyed", "active")
	t.Cleanup(func() {
		killSporeSessions(dirs.project)
		killRoleSessions(dirs.project)
	})

	r, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3})
	if err == nil || !strings.Contains(err.Error(), "assign agent plain") {
		t.Errorf("Reconcile err = %v, want the failed agent assign of plain reported", err)
	}
	if !equalSlices(r.Failed, []string{"plain"}) {
		t.Errorf("Failed = %v, want [plain]", r.Failed)
	}
	spec := filepath.Join(task.RoleTaskDir(dirs.project, "keyed"), "spec.md")
	if _, statErr := os.Stat(spec); statErr != nil {
		t.Errorf("keyed: loop not driven after plain's agent assign failed: %v", statErr)
	}
}
