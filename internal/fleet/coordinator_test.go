package fleet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	spore "github.com/versality/spore"
)

func TestReconcileSpawnsCoordinatorSingleton(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	session := CoordinatorSessionName(dirs.project)

	res, err := Reconcile(Config{
		TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3,
	})
	if err != nil {
		t.Fatalf("Reconcile pass 1: %v", err)
	}
	if !hasSession(session) {
		t.Fatalf("expected coordinator session %q after first reconcile", session)
	}
	if !res.CoordinatorSpawned {
		t.Errorf("pass 1 result does not report the coordinator spawn")
	}

	// Idempotency: a second reconcile must not double-spawn. Capture
	// the session creation timestamp; tmux returns it with #{session_created}.
	ts1, err := sessionCreated(session)
	if err != nil {
		t.Fatalf("sessionCreated #1: %v", err)
	}

	res, err = Reconcile(Config{
		TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3,
	})
	if err != nil {
		t.Fatalf("Reconcile pass 2: %v", err)
	}
	if res.CoordinatorSpawned {
		t.Errorf("pass 2 reports a coordinator spawn; the session was kept")
	}
	if !hasSession(session) {
		t.Fatalf("expected coordinator session %q after second reconcile", session)
	}
	ts2, err := sessionCreated(session)
	if err != nil {
		t.Fatalf("sessionCreated #2: %v", err)
	}
	if ts1 != ts2 {
		t.Errorf("coordinator session was respawned (created %q -> %q); expected idempotent", ts1, ts2)
	}
}

func TestReconcileReapsCoordinatorOnDisable(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	t.Cleanup(func() { killSporeSessions(dirs.project) })

	session := CoordinatorSessionName(dirs.project)

	if _, err := Reconcile(Config{
		TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3,
	}); err != nil {
		t.Fatalf("Reconcile (enable): %v", err)
	}
	if !hasSession(session) {
		t.Fatalf("expected coordinator session %q before disable", session)
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	r, err := Reconcile(Config{
		TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 3,
	})
	if err != nil {
		t.Fatalf("Reconcile (disabled): %v", err)
	}
	if !r.Disabled {
		t.Errorf("expected Disabled=true after Disable(), got %+v", r)
	}
	if hasSession(session) {
		t.Errorf("expected coordinator session %q reaped on flag-disable, still alive", session)
	}
}

func TestReconcileCoordinatorDoesNotCountTowardCap(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")

	for _, slug := range []string{"a", "b"} {
		writeTask(t, dirs.tasks, slug, "active")
	}
	t.Cleanup(func() { killSporeSessions(dirs.project) })

	r, err := Reconcile(Config{
		TasksDir: dirs.tasks, ProjectRoot: dirs.project, MaxWorkers: 2,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, want := r.Spawned, []string{"a", "b"}; !equalSlices(got, want) {
		t.Errorf("Spawned = %v, want %v (coordinator must not consume a worker slot)", got, want)
	}
	if len(r.Skipped) != 0 {
		t.Errorf("Skipped = %v, want []", r.Skipped)
	}

	if !hasSession(CoordinatorSessionName(dirs.project)) {
		t.Errorf("expected coordinator session alive after reconcile")
	}
}

func TestCoordinatorSessionNameUsesMainRepoFromWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	parent := t.TempDir()
	main := filepath.Join(parent, "marketercom")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", main}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	worktree := filepath.Join(main, ".worktrees", "wt-rover-slug")
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", main, "worktree", "add", "-q", worktree, "-b", "wt/rover").CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}

	got := CoordinatorSessionName(worktree)
	want := "spore/marketercom/coordinator"
	if got != want {
		t.Errorf("CoordinatorSessionName(worktree) = %q, want %q", got, want)
	}
}

func TestCoordinatorAgentPrecedence(t *testing.T) {
	cases := []struct {
		name      string
		coordEnv  string
		workerEnv string
		driver    string
		want      string
	}{
		{name: "coord_wins", coordEnv: "agent-A", workerEnv: "agent-B", driver: "codex", want: "agent-A"},
		{name: "worker_fallback", coordEnv: "", workerEnv: "agent-B", driver: "codex", want: "agent-B"},
		{name: "driver_claude", coordEnv: "", workerEnv: "", driver: "claude", want: "claude"},
		{name: "driver_codex", coordEnv: "", workerEnv: "", driver: "codex", want: "codex"},
		{name: "driver_passthrough", coordEnv: "", workerEnv: "", driver: "/usr/local/bin/spore-coordinator-launch", want: "/usr/local/bin/spore-coordinator-launch"},
		{name: "default", coordEnv: "", workerEnv: "", driver: "", want: "claude"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SPORE_COORDINATOR_AGENT", tc.coordEnv)
			t.Setenv("SPORE_AGENT_BINARY", tc.workerEnv)
			cfg := CoordinatorConfig{Driver: tc.driver}
			if got := coordinatorAgent(cfg); got != tc.want {
				t.Errorf("coordinatorAgent(%+v) = %q, want %q", cfg, got, tc.want)
			}
		})
	}
}

func TestEnsureCoordinatorDefersToExternalSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux not available: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spore.toml"),
		[]byte("[coordinator]\nexternal_session_pattern = \"^helm-mcom( \\[.*\\])?$\"\n"),
		0o600); err != nil {
		t.Fatal(err)
	}

	external := "helm-mcom [opus]"
	if err := exec.Command("tmux", "-L", testTmuxSocket, "new-session", "-d", "-s", external, "sleep 86400").Run(); err != nil {
		t.Fatalf("spawn external session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", external).Run()
	})

	got, spawned, err := EnsureCoordinator(dir)
	if err != nil {
		t.Fatalf("EnsureCoordinator: %v", err)
	}
	if spawned {
		t.Errorf("expected spawned=false (external coordinator owns the role), got true")
	}
	if got != external {
		t.Errorf("EnsureCoordinator returned %q, want external %q", got, external)
	}
	if hasSession(CoordinatorSessionName(dir)) {
		t.Errorf("kernel coordinator session %q was spawned despite external match", CoordinatorSessionName(dir))
	}
	if !CoordinatorAlive(dir) {
		t.Errorf("CoordinatorAlive returned false despite matching external session")
	}
}

func TestEnsureCoordinatorPatternNoMatchSpawnsKernel(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skipf("tmux not available: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spore.toml"),
		[]byte("[coordinator]\nexternal_session_pattern = \"^helm-mcom\"\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_COORDINATOR_AGENT", "sleep 30")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", CoordinatorSessionName(dir)).Run()
	})

	got, spawned, err := EnsureCoordinator(dir)
	if err != nil {
		t.Fatalf("EnsureCoordinator: %v", err)
	}
	if !spawned {
		t.Errorf("expected spawned=true when pattern set but no session matches, got false")
	}
	if got != CoordinatorSessionName(dir) {
		t.Errorf("EnsureCoordinator returned %q, want kernel session %q", got, CoordinatorSessionName(dir))
	}
}

// waitForFile polls path until it holds bytes or ~5s pass.
func waitForFile(path string) []byte {
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return b
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// A downstream SPORE_COORDINATOR_AGENT wrapper may reject arguments.
// Before the bundled-role fallback, a project without role.md exec'd it
// with none; it must still get none.
func TestEnsureCoordinatorBundledRoleGivesACustomAgentNoArgs(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(CoordinatorRoleEnv, "")

	out := t.TempDir()
	agent := filepath.Join(t.TempDir(), "strict-launcher")
	script := "#!/bin/sh\necho $# > " + out + "/argc\n" +
		"printf '%s' \"$SPORE_COORDINATOR_ROLE\" > " + out + "/role\n" +
		"[ $# -eq 0 ] || exit 64\nexec sleep 30\n"
	if err := os.WriteFile(agent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_COORDINATOR_AGENT", agent)
	t.Cleanup(func() { killSporeSessions(dirs.project) })

	if _, spawned, err := EnsureCoordinator(dirs.project); err != nil || !spawned {
		t.Fatalf("EnsureCoordinator: spawned=%v err=%v", spawned, err)
	}
	if got := strings.TrimSpace(string(waitForFile(filepath.Join(out, "argc")))); got != "0" {
		t.Fatalf("custom agent got %q args, want 0", got)
	}
	rolePath := string(waitForFile(filepath.Join(out, "role")))
	role, err := os.ReadFile(rolePath)
	if err != nil || string(role) != string(spore.BundledCoordinatorRole) {
		t.Fatalf("SPORE_COORDINATOR_ROLE=%q does not hold the bundled role: %v", rolePath, err)
	}
	if !CoordinatorAlive(dirs.project) {
		t.Fatal("coordinator session did not survive")
	}
}

// Bare claude has no launcher to read $SPORE_COORDINATOR_ROLE, so it
// still gets the bundled role as its first message.
func TestEnsureCoordinatorBundledRoleIsClaudesFirstMessage(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(CoordinatorRoleEnv, "")

	got := filepath.Join(t.TempDir(), "first-message")
	agent := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\neval \"last=\\${$#}\"\nprintf '%s' \"$last\" > " + got + "\nexec sleep 30\n"
	if err := os.WriteFile(agent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_COORDINATOR_AGENT", agent+" --dangerously-skip-permissions")
	t.Cleanup(func() { killSporeSessions(dirs.project) })

	if _, spawned, err := EnsureCoordinator(dirs.project); err != nil || !spawned {
		t.Fatalf("EnsureCoordinator: spawned=%v err=%v", spawned, err)
	}
	first := waitForFile(got)
	want := strings.TrimRight(string(spore.BundledCoordinatorRole), "\n")
	if string(first) != want {
		t.Fatalf("agent first message = %d bytes, want the bundled role (%d bytes)", len(first), len(want))
	}
}

// The spore user's $SHELL is spore-attach, and systemd hands it to the
// reconciler. A tmux server the reconciler starts must not take it as
// default-shell, or the coordinator's command never runs.
func TestEnsureCoordinatorStartsServerWithARealShell(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	tmpdir, err := os.MkdirTemp("", "sfs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpdir) })
	t.Setenv("TMUX_TMPDIR", tmpdir)
	loginShell := filepath.Join(t.TempDir(), "spore-attach")
	if err := os.WriteFile(loginShell, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", loginShell)
	t.Setenv("SPORE_COORDINATOR_AGENT", "sleep 30")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", testTmuxSocket, "kill-server").Run() })

	if _, spawned, err := EnsureCoordinator(dirs.project); err != nil || !spawned {
		t.Fatalf("EnsureCoordinator: spawned=%v err=%v", spawned, err)
	}
}

// A host whose tmux server already runs with spore-attach as its
// default-shell must still get a coordinator: setting SHELL on the
// client only matters when the server starts.
func TestEnsureCoordinatorSpawnsOnAServerWithABrokenDefaultShell(t *testing.T) {
	requireToolchain(t)

	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	tmpdir, err := os.MkdirTemp("", "sfb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpdir) })
	t.Setenv("TMUX_TMPDIR", tmpdir)
	loginShell := filepath.Join(t.TempDir(), "spore-attach")
	if err := os.WriteFile(loginShell, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := exec.Command("tmux", "-L", testTmuxSocket, "-f", "/dev/null", "new-session", "-d", "-s", "keepalive", "sleep", "600")
	start.Env = append(os.Environ(), "SHELL="+loginShell)
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start poisoned server: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", testTmuxSocket, "kill-server").Run() })
	t.Setenv("SPORE_COORDINATOR_AGENT", "sleep 30")

	if _, spawned, err := EnsureCoordinator(dirs.project); err != nil || !spawned {
		t.Fatalf("EnsureCoordinator: spawned=%v err=%v", spawned, err)
	}
}
