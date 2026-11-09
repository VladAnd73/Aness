package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestProbeEnvStripsSporeAndOtherAccounts(t *testing.T) {
	got := probeEnv([]string{"SPORE_TASK_SLUG=coordinator", "SPORE_PROJECT_ROOT=/x", "HOME=/h", "CLAUDECODE=1",
		"CLAUDE_CODE_OAUTH_TOKEN=tok", "ANTHROPIC_API_KEY=k", "ANTHROPIC_AUTH_TOKEN=a", "CLAUDE_CONFIG_DIR=/c"})
	want := []string{"HOME=/h", "CLAUDE_CONFIG_DIR=/c"}
	if !slices.Equal(got, want) {
		t.Errorf("probeEnv: got %v want %v", got, want)
	}
}

func testProbe(t *testing.T, rec *callRecorder, onCall func(p probe, name string, args []string)) probe {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
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

// A default ACL on the state dir overrides the umask, which is how the
// capture came out 0640 on a real host.
func TestProbeCaptureIsPrivateUnderDefaultACL(t *testing.T) {
	setfacl, err := exec.LookPath("setfacl")
	if err != nil {
		t.Skip("setfacl not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command(setfacl, "-d", "-m", "u::rwx,g::r-x,o::---", dir).CombinedOutput(); err != nil {
		t.Skipf("no ACL support here: %v: %s", err, out)
	}
	p := newProbe(filepath.Join(dir, "probe"))
	settings, err := p.writeSettings()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", s.StatusLine.Command)
	cmd.Stdin = strings.NewReader(`{"rate_limits":null}`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("status line command: %v: %s", err, out)
	}
	fi, err := os.Stat(p.captureFile)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("capture mode: got %o want 600", fi.Mode().Perm())
	}
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestProbeRemovesOnlyItsOwnTranscript(t *testing.T) {
	home := t.TempDir()
	stderr := captureDailyStderr(t)
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.environ = func() []string { return []string{"HOME=" + home} }
	p.timeout = 50 * time.Millisecond
	project := filepath.Join(home, ".claude", "projects", strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(p.dir))
	var transcript string
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		if !isCall(args, "new-session") {
			return nil, nil
		}
		id := argAfter(args, "--session-id")
		if id == "" {
			t.Error("claude started without --session-id")
			return nil, nil
		}
		transcript = filepath.Join(project, id+".jsonl")
		for _, f := range []string{transcript, filepath.Join(project, id, "subagent.jsonl"), filepath.Join(project, "other.jsonl"), filepath.Join(project, "memory", "keep.md")} {
			if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
				t.Error(err)
			}
			if err := os.WriteFile(f, []byte("{}\n"), 0o600); err != nil {
				t.Error(err)
			}
		}
		return nil, nil
	})
	_, _ = p.reading(context.Background(), time.Now())
	if transcript == "" {
		t.Fatal("no transcript was created")
	}
	for _, gone := range []string{transcript, strings.TrimSuffix(transcript, ".jsonl")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s survived the probe: %v", gone, err)
		}
	}
	for _, kept := range []string{filepath.Join(project, "other.jsonl"), filepath.Join(project, "memory", "keep.md")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed: %v", kept, err)
		}
	}
	if !strings.Contains(stderr.String(), transcript) {
		t.Errorf("stderr: got %q, want the removed path printed", stderr.String())
	}
}

func TestProbeTranscriptDirNeedsAHome(t *testing.T) {
	for _, env := range [][]string{nil, {"HOME="}, {"CLAUDE_CONFIG_DIR="}} {
		if d := claudeProjectDir(env, "/state/probe"); d != "" {
			t.Errorf("env %q: got %q, want no directory", env, d)
		}
	}
	if d := claudeProjectDir([]string{"HOME=/h", "CLAUDE_CONFIG_DIR=/c"}, "/home/x/.local/state/agent-budget/probe"); d != "/c/projects/-home-x--local-state-agent-budget-probe" {
		t.Errorf("CLAUDE_CONFIG_DIR: got %q", d)
	}
}

func TestReadingsNameTheirCredentials(t *testing.T) {
	u, stateDir := dailyFixture(t)
	u.setWeekly(15, acceptResetsAt)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Fatal(err)
	}
	if rec := readDailyJSON(t, stateDir); rec.Credentials != os.Getenv("AGENT_BUDGET_CREDS") {
		t.Errorf("endpoint credentials: got %q want %q", rec.Credentials, os.Getenv("AGENT_BUDGET_CREDS"))
	}

	weekly := time.Now().Add(48 * time.Hour)
	rec := &callRecorder{}
	p := testProbe(t, rec, func(p probe, name string, args []string) {
		if isCall(args, "send-keys", "Enter") {
			writeCapture(t, p, fmt.Sprintf(`{"rate_limits":{"seven_day":{"used_percentage":15,"resets_at":%d}}}`, weekly.Unix()))
		} else if isCall(args, "new-session") {
			writeCapture(t, p, `{}`)
		}
	})
	r, err := p.reading(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Credentials != "/h/.claude/.credentials.json" {
		t.Errorf("probe credentials: got %q want /h/.claude/.credentials.json", r.Credentials)
	}
}

func TestProbeInterruptIsNotBlamedOnTrust(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := p.reading(ctx, time.Now())
	if err == nil || strings.Contains(err.Error(), "trusted") || !errors.Is(err, context.Canceled) {
		t.Errorf("reading after an interrupt: got %v, want a plain cancellation", err)
	}
}

func TestProbesOnOneSocketDoNotOverlap(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	var mu sync.Mutex
	type window struct{ start, end time.Time }
	windows := make([]window, 2)
	var wg sync.WaitGroup
	for i := range windows {
		p := newProbe(t.TempDir())
		p.environ = func() []string { return []string{"HOME=/h"} }
		p.timeout = 100 * time.Millisecond
		p.poll = 5 * time.Millisecond
		p.run = func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case slices.Contains(args, "new-session"):
				windows[i].start = time.Now()
			case slices.Contains(args, "kill-server"):
				windows[i].end = time.Now()
			}
			return nil, nil
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.reading(context.Background(), time.Now())
		}()
	}
	wg.Wait()
	a, b := windows[0], windows[1]
	if a.start.Before(b.end) && b.start.Before(a.end) {
		t.Errorf("two probes on one tmux socket ran at once: %v-%v and %v-%v", a.start, a.end, b.start, b.end)
	}
}

func TestProbeWaitsForClaudeBeforeRemovingTranscript(t *testing.T) {
	home := t.TempDir()
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.environ = func() []string { return []string{"HOME=" + home} }
	p.timeout = 50 * time.Millisecond
	project := claudeProjectDir([]string{"HOME=" + home}, p.dir)
	var transcript string
	var late *exec.Cmd
	exited := make(chan struct{})
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		switch {
		case isCall(args, "new-session"):
			transcript = filepath.Join(project, argAfter(args, "--session-id")+".jsonl")
			if err := os.MkdirAll(project, 0o700); err != nil {
				t.Error(err)
			}
			late = exec.Command("sh", "-c", `sleep 0.3; echo late >> "$0"`, transcript)
			if err := late.Start(); err != nil {
				t.Error(err)
			}
			go func() { _ = late.Wait(); close(exited) }()
		case isCall(args, "display-message") && strings.Contains(strings.Join(args, " "), "pane_pid"):
			return []byte(fmt.Sprintf("%d\n", late.Process.Pid)), nil
		}
		return nil, nil
	})
	_, _ = p.reading(context.Background(), time.Now())
	<-exited
	if _, err := os.Stat(transcript); !os.IsNotExist(err) {
		t.Errorf("claude wrote %s after it was removed: %v", transcript, err)
	}
}

func TestProbeRemovesTheTranscriptTheStatusLineNamed(t *testing.T) {
	home := t.TempDir()
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.environ = func() []string { return []string{"HOME=" + home} }
	p.timeout = 100 * time.Millisecond
	other := filepath.Join(home, ".claude", "projects", "-hashed-long-name-1a2b3c")
	outside := filepath.Join(home, "elsewhere")
	var named, stranger string
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		if !isCall(args, "new-session") {
			return nil, nil
		}
		id := argAfter(args, "--session-id")
		named = filepath.Join(other, id+".jsonl")
		stranger = filepath.Join(outside, id+".jsonl")
		for _, f := range []string{named, stranger} {
			if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
				t.Error(err)
			}
			if err := os.WriteFile(f, nil, 0o600); err != nil {
				t.Error(err)
			}
		}
		writeCapture(t, p, fmt.Sprintf(`{"transcript_path":%q}`, named))
		return nil, nil
	})
	_, _ = p.reading(context.Background(), time.Now())
	if _, err := os.Stat(named); !os.IsNotExist(err) {
		t.Errorf("transcript named by the status line survived: %v", err)
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("file outside the projects dir was removed: %v", err)
	}
}

func TestProbeTranscriptDirFollowsSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got := claudeProjectDir([]string{"HOME=/h"}, link)
	if want := claudeProjectDir([]string{"HOME=/h"}, real); got != want {
		t.Errorf("symlinked dir: got %q want %q", got, want)
	}
}

func TestProbeIgnoresATranscriptPathOutsideProjects(t *testing.T) {
	home := t.TempDir()
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	p.environ = func() []string { return []string{"HOME=" + home} }
	p.timeout = 100 * time.Millisecond
	var stranger string
	p.run = rec.responder(func(name string, args []string) ([]byte, error) {
		if !isCall(args, "new-session") {
			return nil, nil
		}
		stranger = filepath.Join(home, "elsewhere", argAfter(args, "--session-id")+".jsonl")
		if err := os.MkdirAll(filepath.Dir(stranger), 0o700); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(stranger, nil, 0o600); err != nil {
			t.Error(err)
		}
		writeCapture(t, p, fmt.Sprintf(`{"transcript_path":%q}`, stranger))
		return nil, nil
	})
	_, _ = p.reading(context.Background(), time.Now())
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("a transcript_path outside the projects dir was removed: %v", err)
	}
}
