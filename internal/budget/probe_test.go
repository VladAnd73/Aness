package budget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProbeEnvStripsSpore(t *testing.T) {
	got := probeEnv([]string{"SPORE_TASK_SLUG=coordinator", "SPORE_PROJECT_ROOT=/x", "HOME=/h", "CLAUDECODE=1", "CLAUDE_CODE_OAUTH_TOKEN=tok"})
	want := []string{"HOME=/h", "CLAUDE_CODE_OAUTH_TOKEN=tok"}
	if !slices.Equal(got, want) {
		t.Errorf("probeEnv: got %v want %v", got, want)
	}
}

func testProbe(t *testing.T, rec *callRecorder, onCall func(p probe, name string, args []string)) probe {
	t.Helper()
	p := newProbe(t.TempDir())
	p.environ = func() []string { return []string{"HOME=/h"} }
	p.timeout = 2 * time.Second
	p.poll = 5 * time.Millisecond
	p.run = rec.runner(func(name string, args []string) {
		if onCall != nil {
			onCall(p, name, args)
		}
	})
	return p
}

func writeCapture(t *testing.T, p probe, body string) {
	t.Helper()
	if err := os.WriteFile(p.captureFile, []byte(body), 0o600); err != nil {
		t.Error(err)
	}
}

func isCall(args []string, words ...string) bool {
	for _, w := range words {
		if !slices.Contains(args, w) {
			return false
		}
	}
	return true
}

func TestProbeParsesRateLimits(t *testing.T) {
	weekly := time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)
	five := time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)
	rec := &callRecorder{}
	p := testProbe(t, rec, func(p probe, name string, args []string) {
		switch {
		case isCall(args, "new-session"):
			writeCapture(t, p, `{"rate_limits":null}`)
		case isCall(args, "send-keys", "Enter"):
			writeCapture(t, p, fmt.Sprintf(`{"rate_limits":{"seven_day":{"used_percentage":15,"resets_at":%d},"five_hour":{"used_percentage":3,"resets_at":%d}}}`, weekly.Unix(), five.Unix()))
		}
	})
	r, err := p.reading(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if r.Source != "probe" || r.WeeklyUsed != 15 || !r.WeeklyResetsAt.Equal(weekly) || r.FiveHourUsed != 3 || !r.FiveHourResetsAt.Equal(five) {
		t.Errorf("reading: %+v", r)
	}

	calls := rec.snapshot()
	var sentPrompt bool
	for _, c := range calls {
		if isCall(c.args, "new-session") && !isCall(c.args, "claude", "--settings", "--setting-sources", "project,local") {
			t.Errorf("new-session does not start claude with --settings and without user settings: %v", c.args)
		}
		if isCall(c.args, "send-keys", probePrompt) {
			sentPrompt = true
		}
	}
	if !sentPrompt {
		t.Errorf("prompt never sent: %+v", calls)
	}
	last := calls[len(calls)-1]
	if !slices.Equal(last.args, []string{"-L", "spore-budget-probe", "-f", "/dev/null", "kill-server"}) {
		t.Errorf("last call: got %v want kill-server on the probe socket", last.args)
	}
	settings, err := os.ReadFile(filepath.Join(p.dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settings), "disableAllHooks") || !strings.Contains(string(settings), "statusLine") || !strings.Contains(string(settings), "umask 077") {
		t.Errorf("settings: %s", settings)
	}
}

func TestProbeTimeout(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, func(p probe, name string, args []string) {
		writeCapture(t, p, `{"rate_limits":null}`)
	})
	p.timeout = 50 * time.Millisecond
	if _, err := p.reading(context.Background(), time.Now()); err == nil {
		t.Fatal("reading: want timeout error")
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatal("no calls recorded")
	}
	last := calls[len(calls)-1]
	if !slices.Equal(last.args, []string{"-L", "spore-budget-probe", "-f", "/dev/null", "kill-server"}) {
		t.Errorf("last call: got %v want kill-server on the probe socket", last.args)
	}
}

func TestProbeTearsDownWhenClaudeDies(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.timeout = 5 * time.Second
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		switch {
		case isCall(args, "display-message"):
			return []byte("1 127\n"), nil
		case isCall(args, "capture-pane"):
			return []byte("claude: command not found\nPane is dead (status 127)\n"), nil
		}
		return nil, nil
	})
	start := time.Now()
	_, err := p.reading(context.Background(), time.Now())
	if err == nil || !strings.Contains(err.Error(), "claude: command not found") || !strings.Contains(err.Error(), "status 127") {
		t.Fatalf("reading: got %v, want the dead pane's status and output", err)
	}
	if strings.Contains(err.Error(), "trusted") {
		t.Errorf("error blames folder trust for a dead claude: %v", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("took %v to notice the dead pane; the timeout is %v", waited, p.timeout)
	}
	calls := rec.snapshot()
	if last := calls[len(calls)-1]; !slices.Equal(last.args, []string{"-L", "spore-budget-probe", "-f", "/dev/null", "kill-server"}) {
		t.Errorf("last call: got %v want kill-server", last.args)
	}
}

func TestProbeTearsDownWhenStartFails(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		if slices.Contains(args, "new-session") {
			return nil, errors.New("no tmux server")
		}
		return nil, nil
	})
	if _, err := p.reading(context.Background(), time.Now()); err == nil || !strings.Contains(err.Error(), "no tmux server") {
		t.Fatalf("reading: got %v want the start error", err)
	}
	calls := rec.snapshot()
	if last := calls[len(calls)-1]; !slices.Equal(last.args, []string{"-L", "spore-budget-probe", "-f", "/dev/null", "kill-server"}) {
		t.Errorf("last call: got %v want kill-server", last.args)
	}
}

// Real tmux, fake claude: proves tmux keeps the dead pane and its
// output long enough for the probe to report them.
func TestProbeRealTmuxReportsDeadClaude(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	bin := t.TempDir()
	fake := "#!/bin/sh\necho 'boom: not logged in'\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	p := newProbe(t.TempDir())
	p.socket = realTmuxSocket(t, "spore-budget-probe-test")
	p.environ = func() []string { return []string{"HOME=" + t.TempDir(), "PATH=" + bin + ":" + os.Getenv("PATH")} }
	p.timeout = 10 * time.Second
	p.poll = 20 * time.Millisecond
	start := time.Now()
	_, err := p.reading(context.Background(), time.Now())
	if err == nil || !strings.Contains(err.Error(), "boom: not logged in") || !strings.Contains(err.Error(), "status 3") {
		t.Fatalf("reading: got %v, want the fake claude's output and status", err)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("took %v to notice the dead pane", waited)
	}
	if out, err := exec.Command("tmux", "-L", p.socket, "-f", "/dev/null", "list-sessions").CombinedOutput(); err == nil {
		t.Errorf("probe tmux server still running: %s", out)
	}
}

func TestProbeIgnoresYesterdaysCapture(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.timeout = 100 * time.Millisecond
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(24 * time.Hour).Unix()
	writeCapture(t, p, fmt.Sprintf(`{"rate_limits":{"seven_day":{"used_percentage":1,"resets_at":%d}}}`, stale))
	if r, err := p.reading(context.Background(), time.Now()); err == nil {
		t.Errorf("reading: got %+v from the previous run's capture, want a timeout", r)
	}
}

func TestProbeTmuxSkipsOperatorConfig(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.timeout = 30 * time.Millisecond
	_, _ = p.reading(context.Background(), time.Now())
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatal("no calls")
	}
	for _, c := range calls {
		if c.name == "tmux" && (len(c.args) < 4 || !slices.Equal(c.args[:4], []string{"-L", "spore-budget-probe", "-f", "/dev/null"})) {
			t.Errorf("tmux call may load the operator's config: %v", c.args)
		}
	}
}

func TestProbeKillServerIsBounded(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.timeout = 30 * time.Millisecond
	p.killTimeout = 50 * time.Millisecond
	inner := rec.responder(func(string, []string) ([]byte, error) { return nil, nil })
	p.run = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		if slices.Contains(args, "kill-server") {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return inner(ctx, env, name, args...)
	}
	done := make(chan struct{})
	go func() {
		_, _ = p.reading(context.Background(), time.Now())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reading hung on a kill-server that never returns")
	}
}

func TestDailyContextCancelsOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			guard := make(chan os.Signal, 1)
			signal.Notify(guard, sig)
			defer signal.Stop(guard)
			ctx, stop := dailyContext()
			defer stop()
			if err := syscall.Kill(os.Getpid(), sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Errorf("%v did not cancel the daily context", sig)
			}
		})
	}
}

func TestProbeRealTmuxCancelLeavesNoServer(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := newProbe(t.TempDir())
	p.socket = realTmuxSocket(t, "spore-budget-probe-cancel")
	p.environ = func() []string { return []string{"HOME=" + t.TempDir(), "PATH=" + bin + ":" + os.Getenv("PATH")} }
	p.poll = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := p.reading(ctx, time.Now()); err == nil {
		t.Fatal("reading: want an error after cancel")
	}
	if out, err := exec.Command("tmux", "-L", p.socket, "-f", "/dev/null", "list-sessions").CombinedOutput(); err == nil {
		t.Errorf("probe tmux server survived the cancel: %s", out)
	}
}

// realTmuxSocket kills the test's tmux server and removes its socket
// file, which tmux leaves behind after kill-server.
func realTmuxSocket(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	path := filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), name)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", name, "-f", "/dev/null", "kill-server").Run()
		_ = os.Remove(path)
	})
	return name
}
