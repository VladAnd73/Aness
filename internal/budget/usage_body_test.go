package budget

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEndpointReadingKeepsFiveHour(t *testing.T) {
	u, _ := dailyFixture(t)
	u.setWeekly(15, acceptResetsAt)
	r, err := endpointReading(context.Background(), acceptResetsAt.Add(-5*24*time.Hour))
	if err != nil {
		t.Fatalf("endpointReading: %v", err)
	}
	want := acceptResetsAt.Add(-7*24*time.Hour + 5*time.Hour)
	if r.FiveHourUsed != 3 || !r.FiveHourResetsAt.Equal(want) {
		t.Errorf("five hour: got %v at %v, want 3 at %v", r.FiveHourUsed, r.FiveHourResetsAt, want)
	}
}

func TestEndpointReadingToleratesOddLimits(t *testing.T) {
	reset := acceptResetsAt.Format(time.RFC3339)
	for name, limits := range map[string]string{
		"object not list":   `{"kind":"weekly_scoped"}`,
		"percent as string": `[{"kind":"weekly_scoped","percent":"7","scope":{"model":{"display_name":"Fable"}}}]`,
		"kind as number":    `[{"kind":3}]`,
	} {
		t.Run(name, func(t *testing.T) {
			u, stateDir := dailyFixture(t)
			u.set(http.StatusOK, `{"seven_day":{"utilization":15,"resets_at":"`+reset+`"},"limits":`+limits+`}`)
			if _, err := runDaily(context.Background(), acceptResetsAt.Add(-5*24*time.Hour), []ReadingSource{endpointReading}); err != nil {
				t.Fatalf("runDaily: %v", err)
			}
			rec := readDailyJSON(t, stateDir)
			if rec.Verdict != VerdictRoom || rec.Source != "endpoint" {
				t.Errorf("record: verdict %q source %q, want room from the endpoint", rec.Verdict, rec.Source)
			}
			if len(rec.Errors) != 1 || !strings.Contains(rec.Errors[0], "limits") {
				t.Errorf("errors: got %q, want one note about limits", rec.Errors)
			}
		})
	}
}

func usageRefreshFixture(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	creds := filepath.Join(dir, ".credentials.json")
	writeCredentials(t, creds, "at-fresh", "", time.Now().Add(2*time.Hour).UnixMilli())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	swapURLs(t, srv.URL, srv.URL+"/token-unused")
	t.Setenv("AGENT_BUDGET_PROJECTS", filepath.Join(dir, "projects"))
	t.Setenv("AGENT_BUDGET_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("AGENT_BUDGET_CREDS", creds)
	t.Setenv("AGENT_BUDGET_ACCOUNTS_DIR", filepath.Join(dir, "no-accounts"))
	t.Setenv("AGENT_BUDGET_USAGE_MIN_INTERVAL_SEC", "0")
}

func TestRefreshTreatsMissingSevenDayAsStaleNotZero(t *testing.T) {
	for name, second := range map[string]string{
		"null":             `{"five_hour":{"utilization":50,"resets_at":"2026-04-26T17:00:00.000000+00:00"},"seven_day":null}`,
		"absent":           `{"five_hour":{"utilization":50,"resets_at":"2026-04-26T17:00:00.000000+00:00"}}`,
		"utilization null": `{"seven_day":{"utilization":null,"resets_at":"2026-04-26T20:00:00.000000+00:00"}}`,
		"out of range":     `{"seven_day":{"utilization":-5,"resets_at":"2026-04-26T20:00:00.000000+00:00"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var calls int32
			usageRefreshFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&calls, 1) == 1 {
					_, _ = fmt.Fprintln(w, `{"five_hour":{"utilization":50,"resets_at":"2026-04-26T17:00:00.000000+00:00"},"seven_day":{"utilization":25,"resets_at":"2026-04-26T20:00:00.000000+00:00"}}`)
					return
				}
				_, _ = fmt.Fprintln(w, second)
			})
			if err := Refresh(); err != nil {
				t.Fatalf("first refresh: %v", err)
			}
			if err := Refresh(); err != nil {
				t.Fatalf("second refresh: %v", err)
			}
			s, err := loadState()
			if err != nil {
				t.Fatal(err)
			}
			if s.UsageSnapshot == nil || !s.UsageSnapshot.Stale {
				t.Fatalf("snapshot: got %+v, want the first reading marked stale", s.UsageSnapshot)
			}
			if math.Abs(s.Long.Frac-0.25) > 1e-9 || s.Long.Source != "usage-stale" {
				t.Errorf("long: frac %.4f source %q, want 0.25 usage-stale", s.Long.Frac, s.Long.Source)
			}
		})
	}
}

func TestRefreshSkipsAccountWithoutSevenDay(t *testing.T) {
	dir := t.TempDir()
	accountsD := filepath.Join(dir, "accounts")
	writeAccountFile(t, accountsD, "primary", "at-primary", "")
	writeAccountFile(t, accountsD, "secondary", "at-secondary", "")
	usageByToken := map[string]string{
		"at-primary":   `{"five_hour":{"utilization":80,"resets_at":"2026-04-26T17:00:00.000000+00:00"},"seven_day":{"utilization":60,"resets_at":"2026-04-26T20:00:00.000000+00:00"}}`,
		"at-secondary": `{"five_hour":{"utilization":20,"resets_at":"2026-04-26T17:00:00.000000+00:00"},"seven_day":null}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, usageByToken[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")])
	}))
	defer srv.Close()
	swapURLs(t, srv.URL, srv.URL+"/token-unused")
	t.Setenv("AGENT_BUDGET_PROJECTS", filepath.Join(dir, "projects-empty"))
	t.Setenv("AGENT_BUDGET_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("AGENT_BUDGET_ACCOUNTS_DIR", accountsD)
	t.Setenv("AGENT_BUDGET_USAGE_MIN_INTERVAL_SEC", "0")
	if err := Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.AccountSnapshots["secondary"]; ok {
		t.Errorf("secondary snapshot kept from a body without seven_day: %+v", s.AccountSnapshots["secondary"])
	}
	if math.Abs(s.Long.Frac-0.60) > 1e-9 {
		t.Errorf("aggregate long.frac: got %.4f want 0.60 (secondary must not count as 0%%)", s.Long.Frac)
	}
}

func TestRefreshKeepsAnExhaustedAccount(t *testing.T) {
	usageRefreshFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"five_hour":{"utilization":40,"resets_at":"2026-04-26T17:00:00.000000+00:00"},"seven_day":{"utilization":100.4,"resets_at":"2026-04-26T20:00:00.000000+00:00"}}`)
	})
	if err := Refresh(); err != nil {
		t.Fatal(err)
	}
	s, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if s.UsageSnapshot == nil || s.UsageSnapshot.Stale || s.UsageSnapshot.Long.Utilization != 100.4 || s.UsageSnapshot.Short.Utilization != 40 {
		t.Errorf("snapshot: got %+v, want a fresh 100.4%% weekly with its five hour data", s.UsageSnapshot)
	}
}
