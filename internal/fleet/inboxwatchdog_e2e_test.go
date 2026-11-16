package fleet

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/versality/spore/internal/hooks"
	"github.com/versality/spore/internal/task"
)

// Scenario 9, end to end. This is the 2026-10-06 incident replayed
// against a coordinator-shaped session in a temp project: an envelope
// lands, the real Stop-hook drain claims and prints it, and the
// re-prompt it asked for never arrives. One real Reconcile tick must
// put the envelope back in the inbox and leave a live watcher behind,
// with no operator action anywhere in the loop.
//
// Everything here is the production path: the real spore binary, the
// real drain, the real inotify watcher, the real Reconcile. Two things
// are simulated, and only two: the dropped re-prompt (by simply not
// running a second hook, which is exactly what a dropped re-prompt
// looks like from disk) and the passage of the grace window (by
// backdating the record's claim time).
func TestReconcile_RecoversLostWakeupEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the spore binary")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	home := t.TempDir()
	root := filepath.Join(home, "proj")
	tasks := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))

	sporeBin := buildSpore(t, filepath.Join(home, "bin"))
	t.Setenv("PATH", filepath.Dir(sporeBin)+":"+os.Getenv("PATH"))

	poke, err := task.CoordinatorInboxDirForProject(root)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := task.InboxDirForProject(root, CoordinatorSlug)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{poke, msg} {
		if err := os.MkdirAll(filepath.Join(d, "read"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// The coordinator session: alive, and idle in the sense that
	// matters -- its transcript has not been written since before the
	// envelope ever arrived. Its pane shows an agent prompt with
	// nothing typed at it, which is the only state a nudge may touch.
	session := CoordinatorSessionName(root)
	startIdleSession(t, home, session)
	writeStaleTranscript(t, home, root)

	if err := Enable(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Disable() })

	// A watcher relays a new-thread envelope into the message inbox.
	envelope := filepath.Join(msg, "100-1-1.json")
	if err := os.WriteFile(envelope,
		[]byte(`{"ts":"2026-10-06T10:06:23Z","source":"slack-watcher","body":"new thread"}`),
		0o600); err != nil {
		t.Fatal(err)
	}

	// The real Stop hook drains it and asks for a re-prompt (exit 2).
	out, code := runSporeHook(t, sporeBin, root, poke)
	if code != 2 {
		t.Fatalf("watch-inbox exit = %d, want 2 (output: %q)", code, out)
	}
	if !strings.Contains(out, "new thread") {
		t.Fatalf("drain did not print the envelope: %q", out)
	}
	if _, err := os.Stat(envelope); !os.IsNotExist(err) {
		t.Fatal("envelope should have been claimed out of the inbox")
	}

	// The re-prompt is dropped: nothing runs a second hook, so the
	// record is never acknowledged. Age it past the grace window.
	backdateClaim(t, msg, inboxWatchdogGrace+time.Minute)

	res, err := Reconcile(Config{TasksDir: tasks, ProjectRoot: root, MaxWorkers: 1})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if res.InboxWatchdog.Requeued != 1 {
		t.Errorf("Requeued = %d, want 1 (skipped: %q)",
			res.InboxWatchdog.Requeued, res.InboxWatchdog.Skipped)
	}
	if _, err := os.Stat(envelope); err != nil {
		t.Errorf("envelope not back in the inbox: %v", err)
	}
	if res.InboxWatchdog.Escalated {
		t.Error("tick 1 escalated to the pane")
	}

	pid := liveObserverPid(t, root)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	if !res.InboxWatchdog.Rearmed {
		t.Error("Rearmed = false")
	}

	// Nobody touches anything. The re-arm alone cannot deliver the
	// envelope, because the agent is not taking turns -- that is what
	// a lost wakeup is. After the configured number of quiet ticks the
	// watchdog stops being silent and tells the operator, once.
	nudged := 0
	for tick := 2; tick <= DefaultInboxWatchdogEscalateAfter+1; tick++ {
		res, err = Reconcile(Config{TasksDir: tasks, ProjectRoot: root, MaxWorkers: 1})
		if err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
		if res.InboxWatchdog.Escalated {
			nudged++
			if tick != DefaultInboxWatchdogEscalateAfter {
				t.Errorf("escalated at tick %d, want tick %d",
					tick, DefaultInboxWatchdogEscalateAfter)
			}
		}
	}
	if nudged != 1 {
		t.Fatalf("escalated %d time(s) across the run, want exactly 1 (last skip: %q)",
			nudged, res.InboxWatchdog.Skipped)
	}
	if pane := captureTestPane(t, session); !strings.Contains(pane, "requeued") {
		t.Errorf("the nudge never reached the pane: %q", pane)
	}

	// The envelope is deliverable again: a fresh hook run drains it
	// and asks for a wake, exactly as it would have on 2026-10-06 had
	// the agent taken any turn.
	out, code = runSporeHook(t, sporeBin, root, poke)
	if code != 2 || !strings.Contains(out, "new thread") {
		t.Fatalf("recovered envelope not redeliverable: exit=%d out=%q", code, out)
	}
}

func captureTestPane(t *testing.T, session string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", testTmuxSocket, "capture-pane", "-p", "-t", session).Output()
	if err != nil {
		t.Fatalf("capture-pane %s: %v", session, err)
	}
	return string(out)
}

func buildSpore(t *testing.T, binDir string) string {
	t.Helper()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "spore")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/spore")
	cmd.Dir = repoRootForTest(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	return bin
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd)) // internal/fleet -> repo root
}

// runSporeHook runs the real Stop-hook watcher with the env a
// coordinator pane carries. WATCH_TIMEOUT=1 keeps a run that finds
// nothing from blocking the test.
func runSporeHook(t *testing.T, bin, root, poke string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, "hooks", "watch-inbox")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"SPORE_TASK_SLUG="+CoordinatorSlug,
		"SPORE_PROJECT_ROOT="+root,
		"SPORE_TASK_INBOX="+poke,
		"WATCH_TIMEOUT=1",
		"WATCH_SETTLE=0",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !asExitError(err, &ee) {
			t.Fatalf("watch-inbox: %v: %s", err, out)
		}
		code = ee.ExitCode()
	}
	return string(out), code
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// startIdleSession brings up a coordinator-named tmux session on the
// test socket whose pane ends in an agent prompt with nothing typed at
// it: a live session, a readable pane, no operator draft. That is the
// one pane state the watchdog is allowed to type into, so it is the
// state the escalation has to be proved against.
func startIdleSession(t *testing.T, home, session string) {
	t.Helper()
	script := filepath.Join(home, "idle-pane.sh")
	body := "#!/bin/sh\ni=0\nwhile [ $i -lt 30 ]; do echo; i=$((i+1)); done\nprintf '\\342\\235\\257\\302\\240'\nsleep 86400\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("tmux", "-L", testTmuxSocket, "new-session", "-d", "-s", session, script).Run(); err != nil {
		t.Fatalf("tmux new-session %s: %v", session, err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", testTmuxSocket, "kill-session", "-t", session).Run() })
	for range 40 {
		if strings.Contains(captureTestPane(t, session), "❯") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pane never showed a prompt: %q", captureTestPane(t, session))
}

// writeStaleTranscript gives the project an agent transcript whose last
// write is an hour old: the liveness probe can answer, and its answer
// is "this session has done nothing recently".
func writeStaleTranscript(t *testing.T, home, root string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(root))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
}

// backdateClaim rewrites the record's claim time to age it past the
// watchdog's grace window without sleeping.
func backdateClaim(t *testing.T, inbox string, by time.Duration) {
	t.Helper()
	rec, ok, err := hooks.ReadPendingAck(inbox)
	if err != nil || !ok {
		t.Fatalf("no record to backdate: ok=%v err=%v", ok, err)
	}
	rec.ClaimedAt = rec.ClaimedAt.Add(-by)
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooks.PendingAckPath(inbox), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// liveObserverPid asserts the re-armed watcher is a real running
// process, not just a flag in a struct.
func liveObserverPid(t *testing.T, root string) int {
	t.Helper()
	p, err := observerPidPath(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("no observer pidfile: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("bad pidfile %q: %v", b, err)
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		t.Fatalf("observer pid %d is not running: %v", pid, err)
	}
	if !strings.Contains(strings.ReplaceAll(string(cmdline), "\x00", " "), "watch-inbox --observe") {
		t.Fatalf("pid %d is not an inbox observer: %q", pid, cmdline)
	}
	return pid
}
