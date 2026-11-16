package fleet

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

func writeAgentEnv(t *testing.T, root, body string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	project, err := task.ProjectName(root)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfg, "spore", project, "agent.env")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRoleSpawnSpecCarriesAgentEnv(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	writeAgentEnv(t, root, "FOO=bar\nSPORE_ROLE=x\n")

	args, err := EngineerSpec(root, "feature-x").TmuxArgs()
	if err != nil {
		t.Fatalf("TmuxArgs: %v", err)
	}
	foo := slices.Index(args, "FOO=bar")
	if foo < 1 || args[foo-1] != "-e" {
		t.Fatalf("args = %q, want -e FOO=bar", args)
	}
	role := slices.Index(args, "SPORE_ROLE=engineer")
	if role < 0 || foo < role {
		t.Errorf("FOO=bar at %d, kernel SPORE_ROLE at %d: want project env after the kernel -e flags", foo, role)
	}
	if slices.Contains(args, "SPORE_ROLE=x") {
		t.Errorf("reserved SPORE_ROLE=x leaked into args: %q", args)
	}
}

func writeIsolateNetwork(t *testing.T, root string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte("[worker]\nisolate_network = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pathWith returns a PATH holding only git plus the named fake
// binaries, so the host's own pasta cannot leak into the test.
func pathWith(t *testing.T, fakes ...string) string {
	t.Helper()
	dir := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	for _, f := range fakes {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRoleSpawnSpecIsolationWrap(t *testing.T) {
	t.Run("pasta present", func(t *testing.T) {
		root := newGitRepoFor(t, "demo-project")
		writeIsolateNetwork(t, root)
		t.Setenv("PATH", pathWith(t, "pasta"))
		args, err := EngineerSpec(root, "feature-x").TmuxArgs()
		if err != nil {
			t.Fatalf("TmuxArgs: %v", err)
		}
		last := args[len(args)-1]
		if strings.Count(last, "pasta --config-net") != 2 {
			t.Errorf("shell command = %q, want both invocations wrapped in pasta --config-net", last)
		}
	})
	t.Run("pasta absent", func(t *testing.T) {
		root := newGitRepoFor(t, "demo-project")
		writeIsolateNetwork(t, root)
		t.Setenv("PATH", pathWith(t))
		_, err := EngineerSpec(root, "feature-x").TmuxArgs()
		if err == nil || !strings.Contains(err.Error(), "isolate_network") {
			t.Errorf("TmuxArgs err = %v, want an isolate_network error", err)
		}
	})
}

// Role notices must land where the coordinator's watcher looks even
// when the watcher resolves state through XDG_STATE_HOME and no
// SPORE_COORDINATOR_STATE_DIR is set.
func TestRoleLoopNoticeLandsInWatchedInbox(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", "")
	os.Unsetenv("SPORE_COORDINATOR_STATE_DIR")

	notifyRoleLoop(root, "demo: hello")

	dirs, err := coordinatorInboxDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		for _, e := range readInboxEnvelopes(t, d) {
			if e.Body == "demo: hello" {
				return
			}
		}
	}
	t.Errorf("notice not in any watched inbox %v", dirs)
}

func TestRoleSpawnBackoffPersistsAcrossReconcile(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	t.Cleanup(func() { killLoopTestSessions(filepath.Base(dirs.project)) })

	now := time.Now()
	b := loadRoleSpawnBackoff(dirs.project)
	b.failed("demo/engineer", now)
	if err := b.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if d := loadRoleSpawnBackoff(dirs.project).wait("demo/engineer", now); d <= 0 {
		t.Errorf("role backoff after Reconcile: wait = %s, want > 0 (record lost)", d)
	}
}

// A token-monitor wrap kills its own session and leaves no dead-agent
// marker. The driver must bring the pane back silently, outside the
// respawn cap, and reconcile must not start a generic worker for the
// slug.
func TestRolePaneTokenWrapIsRespawnedByDriver(t *testing.T) {
	root, inbox := livenessFixture(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	tasksDir := setupRoleLoopDriveFixture(t, root, "demo")
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte("[fleet]\nrole_stall_minutes = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := stubPanes(t)
	prevGate := roleSpawnGate
	roleSpawnGate = realRoleSpawnGate
	t.Cleanup(func() { roleSpawnGate = prevGate })

	if _, err := DriveRoleLoop(root, tasksDir, "demo"); err != nil {
		t.Fatal(err)
	}
	eng, _ := EngineerSpec(root, "demo").SessionName()
	f.now = f.now.Add(quickDeathWindow + time.Minute)
	if _, err := DriveRoleLoop(root, tasksDir, "demo"); err != nil {
		t.Fatal(err)
	}
	for wrap := 1; wrap <= MaxRoleRespawns+1; wrap++ {
		delete(f.panes, eng)
		if _, err := DriveRoleLoop(root, tasksDir, "demo"); err != nil {
			t.Fatal(err)
		}
		if f.panes[eng] == nil {
			t.Fatalf("wrap %d: engineer not respawned; spawns = %v", wrap, f.spawns)
		}
		f.now = f.now.Add(quickDeathWindow + time.Minute)
		if _, err := DriveRoleLoop(root, tasksDir, "demo"); err != nil {
			t.Fatal(err)
		}
	}
	if bodies := envBodies(t, inbox); len(bodies) != 0 {
		t.Errorf("token-wrap respawns notified: %q", bodies)
	}
	stateDir := filepath.Join(task.RoleTaskDir(root, "demo"), "state")
	if n, _ := countMarkers(stateDir, "respawn-"); n != 0 {
		t.Errorf("token-wrap respawns wrote %d respawn markers, want 0", n)
	}

	t.Run("reconcile spawns no generic worker", func(t *testing.T) {
		requireToolchain(t)
		mustEnable(t)
		t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
		t.Cleanup(func() { killLoopTestSessions(filepath.Base(root)) })
		r, err := Reconcile(Config{TasksDir: tasksDir, ProjectRoot: root, MaxWorkers: 3})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if slices.Contains(r.Spawned, "demo") {
			t.Errorf("Reconcile spawned a generic worker for role task demo: %+v", r)
		}
	})
}

func TestRoleSpawnGateBacksOffQuickDeaths(t *testing.T) {
	root, _ := livenessFixture(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	f := stubPanes(t)
	prevGate := roleSpawnGate
	roleSpawnGate = realRoleSpawnGate
	t.Cleanup(func() { roleSpawnGate = prevGate })
	spec := EngineerSpec(root, "demo")
	session, _ := spec.SessionName()

	for range 3 {
		if err := ensureRolePane(spec, 0); err != nil {
			t.Fatal(err)
		}
		delete(f.panes, session)
	}
	if len(f.spawns) != 1 {
		t.Errorf("spawns = %d at one instant, want 1 (backoff)", len(f.spawns))
	}
	keys := spawnRecordKeys(t, root)
	if !slices.Contains(keys, "demo/engineer") {
		t.Errorf("backoff keys = %v, want demo/engineer", keys)
	}
	raw, _ := os.ReadFile(roleSpawnsPathForTest(t, root))
	var recs map[string]json.RawMessage
	if json.Unmarshal(raw, &recs) != nil || recs["demo/engineer"] == nil {
		t.Errorf("role-spawns.json = %s, want a demo/engineer record", raw)
	}
}

func roleSpawnsPathForTest(t *testing.T, root string) string {
	t.Helper()
	dir, err := task.StateDirForProject(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "role-spawns.json")
}
