package budget

import (
	"context"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestComputeVerdictRoom(t *testing.T) {
	resets := acceptResetsAt
	now := resets.Add(-5 * 24 * time.Hour)
	rec := computeVerdict(Reading{Source: "endpoint", WeeklyUsed: 15, WeeklyResetsAt: resets}, 10, now)
	if math.Abs(rec.Pace-28.571) > 0.01 {
		t.Errorf("pace: got %v want ~28.571", rec.Pace)
	}
	if math.Abs(rec.Headroom-13.571) > 0.01 {
		t.Errorf("headroom: got %v want ~13.571", rec.Headroom)
	}
	if rec.Verdict != VerdictRoom {
		t.Errorf("verdict: got %q want room", rec.Verdict)
	}
	if !rec.TakenAt.Equal(now) || rec.Source != "endpoint" || rec.Margin != 10 {
		t.Errorf("record fields: %+v", rec)
	}
}

func TestComputeVerdictNoRoom(t *testing.T) {
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	rec := computeVerdict(Reading{WeeklyUsed: 25, WeeklyResetsAt: acceptResetsAt}, 10, now)
	if rec.Verdict != VerdictNoRoom {
		t.Errorf("verdict: got %q want no-room", rec.Verdict)
	}
}

func TestComputeVerdictBoundary(t *testing.T) {
	week := 7 * 24 * time.Hour
	now := acceptResetsAt.Add(-week).Add(week * 7 / 10)
	rec := computeVerdict(Reading{WeeklyUsed: 60, WeeklyResetsAt: acceptResetsAt}, 10, now)
	if math.Abs(rec.Headroom-10) > 1e-9 {
		t.Fatalf("headroom: got %v want exactly 10", rec.Headroom)
	}
	if rec.Verdict != VerdictRoom {
		t.Errorf("verdict at headroom == margin: got %q want room", rec.Verdict)
	}
}

func TestComputeVerdictClampsElapsed(t *testing.T) {
	r := Reading{WeeklyUsed: 0, WeeklyResetsAt: acceptResetsAt}
	if rec := computeVerdict(r, 10, acceptResetsAt.Add(time.Hour)); rec.Pace != 100 {
		t.Errorf("after reset: pace %v want 100", rec.Pace)
	}
	if rec := computeVerdict(r, 10, acceptResetsAt.Add(-8*24*time.Hour)); rec.Pace != 0 {
		t.Errorf("before window: pace %v want 0", rec.Pace)
	}
}

func TestComputeVerdictCopiesFiveHourAndExtra(t *testing.T) {
	five := acceptResetsAt.Add(-6 * 24 * time.Hour)
	rec := computeVerdict(Reading{
		WeeklyUsed: 1, WeeklyResetsAt: acceptResetsAt,
		FiveHourUsed: 3, FiveHourResetsAt: five,
		Extra: map[string]float64{"Fable": 4},
	}, 10, acceptResetsAt.Add(-6*24*time.Hour))
	if rec.FiveHourUsed != 3 || !rec.FiveHourResetsAt.Equal(five) || rec.Extra["Fable"] != 4 || !rec.WeeklyResetsAt.Equal(acceptResetsAt) {
		t.Errorf("record fields: %+v", rec)
	}
}

func TestDailyWritesHistoryAndMode(t *testing.T) {
	u, stateDir := dailyFixture(t)
	u.setWeekly(15, acceptResetsAt)
	if _, err := runDaily(context.Background(), acceptResetsAt.Add(-5*24*time.Hour), []ReadingSource{endpointReading}); err != nil {
		t.Fatalf("runDaily: %v", err)
	}
	hist, err := os.ReadFile(filepath.Join(stateDir, "daily-history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(hist), "\n"); n != 1 {
		t.Errorf("history lines: got %d want 1", n)
	}
	for _, name := range []string{"daily.json", "daily-history.jsonl"} {
		fi, err := os.Stat(filepath.Join(stateDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode: got %o want 600", name, fi.Mode().Perm())
		}
	}
	if fi, err := os.Stat(stateDir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("state dir mode: %v %v", fi.Mode().Perm(), err)
	}
}

func TestDailyEndpointMissingSevenDay(t *testing.T) {
	reset := acceptResetsAt.Format(time.RFC3339)
	for name, body := range map[string]string{
		"absent":          `{"five_hour":{"utilization":3,"resets_at":"` + reset + `"}}`,
		"null":            `{"five_hour":{"utilization":3,"resets_at":"` + reset + `"},"seven_day":null}`,
		"resets null":     `{"seven_day":{"utilization":15,"resets_at":null}}`,
		"resets empty":    `{"seven_day":{"utilization":15,"resets_at":""}}`,
		"resets garbage":  `{"seven_day":{"utilization":15,"resets_at":"soon"}}`,
		"utilization nil": `{"seven_day":{"utilization":null,"resets_at":"` + reset + `"}}`,
		"not json":        `<html>`,
	} {
		t.Run(name, func(t *testing.T) {
			u, stateDir := dailyFixture(t)
			u.set(http.StatusOK, body)
			_, err := runDaily(context.Background(), acceptResetsAt.Add(-5*24*time.Hour), []ReadingSource{endpointReading})
			if err == nil {
				t.Error("runDaily: want error")
			}
			if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown || rec.Error == "" {
				t.Errorf("record: verdict %q error %q, want unknown with an error", rec.Verdict, rec.Error)
			}
		})
	}
}

func TestEndpointReadingPerModelExtra(t *testing.T) {
	u, _ := dailyFixture(t)
	reset := acceptResetsAt.Format(time.RFC3339)
	u.set(http.StatusOK, `{"five_hour":null,"seven_day":{"utilization":23,"resets_at":"`+reset+`"},`+
		`"limits":[{"kind":"weekly_all","percent":23},`+
		`{"kind":"weekly_scoped","percent":7,"scope":{"model":{"display_name":"Fable"}}},`+
		`{"kind":"weekly_scoped","percent":null,"scope":null}]}`)
	r, err := endpointReading(context.Background(), acceptResetsAt.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("endpointReading: %v", err)
	}
	if r.WeeklyUsed != 23 || !r.WeeklyResetsAt.Equal(acceptResetsAt) || r.Source != "endpoint" {
		t.Errorf("reading: %+v", r)
	}
	if len(r.Extra) != 1 || r.Extra["Fable"] != 7 {
		t.Errorf("extra: got %v want map[Fable:7]", r.Extra)
	}
}

func TestDailyBadConfig(t *testing.T) {
	u, stateDir := dailyFixture(t)
	u.setWeekly(15, acceptResetsAt)
	if err := os.WriteFile(os.Getenv("SPORE_BUDGET_CONFIG"), []byte("margin = -1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runDaily(context.Background(), acceptResetsAt.Add(-5*24*time.Hour), []ReadingSource{endpointReading}); err == nil {
		t.Error("runDaily: want config error")
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown || !strings.Contains(rec.Error, "margin") {
		t.Errorf("record after bad config: verdict %q error %q, want unknown naming the config error", rec.Verdict, rec.Error)
	}
}

func TestDailyUnknownJoinsSourceErrors(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	_, err := runDaily(context.Background(), now, []ReadingSource{failingSource("first broke"), failingSource("second broke")})
	if err == nil || !strings.Contains(err.Error(), "first broke") || !strings.Contains(err.Error(), "second broke") {
		t.Errorf("error: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Verdict != VerdictUnknown || !rec.TakenAt.Equal(now) || rec.Margin != 10 {
		t.Errorf("record: %+v", rec)
	}
}

func TestDailyRejectsImplausibleReset(t *testing.T) {
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	for name, resets := range map[string]time.Time{
		"past":       now.Add(-time.Hour),
		"epoch zero": time.Unix(0, 0).UTC(),
		"beyond 7d":  now.Add(8 * 24 * time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			_, stateDir := dailyFixture(t)
			bad := fixedReading(Reading{Source: "endpoint", WeeklyUsed: 1, WeeklyResetsAt: resets})
			if _, err := runDaily(context.Background(), now, []ReadingSource{bad}); err == nil {
				t.Error("runDaily: want error for an implausible weekly reset")
			}
			if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown {
				t.Errorf("verdict: got %q want unknown", rec.Verdict)
			}

			good := fixedReading(Reading{Source: "probe", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt})
			if _, err := runDaily(context.Background(), now, []ReadingSource{bad, good}); err != nil {
				t.Fatalf("runDaily with fallback: %v", err)
			}
			if rec := readDailyJSON(t, stateDir); rec.Source != "probe" || rec.Verdict != VerdictRoom {
				t.Errorf("fallback record: %+v", rec)
			}
		})
	}
}
