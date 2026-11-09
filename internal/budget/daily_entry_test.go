package budget

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func fakeProbeRunner(t *testing.T, stateDir string, weekly time.Time, called *bool) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	capture := filepath.Join(stateDir, "probe", "statusline.json")
	old := probeRunner
	probeRunner = func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		*called = true
		body := ""
		switch {
		case slices.Contains(args, "new-session"):
			body = `{}`
		case slices.Contains(args, "Enter"):
			body = fmt.Sprintf(`{"rate_limits":{"seven_day":{"used_percentage":20,"resets_at":%d}}}`, weekly.Unix())
		}
		if body != "" {
			if err := os.WriteFile(capture, []byte(body), 0o600); err != nil {
				t.Error(err)
			}
		}
		return nil, nil
	}
	t.Cleanup(func() { probeRunner = old })
}

func TestDailyUsesEndpointFirst(t *testing.T) {
	u, stateDir := dailyFixture(t)
	weekly := time.Now().Add(3 * 24 * time.Hour).UTC()
	u.setWeekly(15, weekly)
	var probed bool
	fakeProbeRunner(t, stateDir, weekly, &probed)
	if err := Daily(); err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if rec := readDailyJSON(t, stateDir); rec.Source != "endpoint" || rec.WeeklyUsed != 15 {
		t.Errorf("record: source %q used %v, want the endpoint's 15", rec.Source, rec.WeeklyUsed)
	}
	if probed {
		t.Error("probe ran although the endpoint answered")
	}
}

func TestDailyFallsBackToProbe(t *testing.T) {
	u, stateDir := dailyFixture(t)
	weekly := time.Now().Add(3 * 24 * time.Hour).UTC()
	u.set(http.StatusServiceUnavailable, "down")
	var probed bool
	fakeProbeRunner(t, stateDir, weekly, &probed)
	if err := Daily(); err != nil {
		t.Fatalf("Daily: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Source != "probe" || rec.WeeklyUsed != 20 || !probed {
		t.Errorf("record: source %q used %v probed %v, want the probe's 20", rec.Source, rec.WeeklyUsed, probed)
	}
	if len(rec.Errors) != 1 || !strings.Contains(rec.Errors[0], "HTTP 503") {
		t.Errorf("errors: got %q, want the endpoint failure", rec.Errors)
	}
}
