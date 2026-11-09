package budget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestProbeTearsDownWhenStartFails(t *testing.T) {
	rec := &callRecorder{}
	p := testProbe(t, rec, nil)
	inner := p.run
	p.run = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		_, _ = inner(ctx, env, name, args...)
		if slices.Contains(args, "new-session") {
			return nil, errors.New("no claude")
		}
		return nil, nil
	}
	if _, err := p.reading(context.Background(), time.Now()); err == nil || !strings.Contains(err.Error(), "no claude") {
		t.Fatalf("reading: got %v want the start error", err)
	}
	calls := rec.snapshot()
	last := calls[len(calls)-1]
	if !slices.Equal(last.args, []string{"-L", "spore-budget-probe", "-f", "/dev/null", "kill-server"}) {
		t.Errorf("last call: got %v want kill-server", last.args)
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
