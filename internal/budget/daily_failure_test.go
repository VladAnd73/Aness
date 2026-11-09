package budget

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func yesterdayRoom(t *testing.T, stateDir string, now time.Time) {
	t.Helper()
	writeDailyJSON(t, stateDir, DailyRecord{
		TakenAt: now.Add(-24 * time.Hour), Source: "endpoint", Verdict: VerdictRoom,
		WeeklyUsed: 5, WeeklyResetsAt: acceptResetsAt, Pace: 28.6, Headroom: 23.6, Margin: 10,
	})
}

func TestBadConfigReplacesYesterdaysRoomWithUnknown(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-4 * 24 * time.Hour)
	yesterdayRoom(t, stateDir, now)
	u.setWeekly(5, acceptResetsAt)
	if err := os.WriteFile(os.Getenv("SPORE_BUDGET_CONFIG"), []byte("margin = 10%\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err == nil {
		t.Fatal("want config error")
	}
	if code, line := roomVerdict(now.Add(time.Minute)); code != 2 {
		t.Errorf("room after a bad-config daily run: got %d (%q) want 2", code, line)
	}
	if rec := readDailyJSON(t, stateDir); rec.Margin != defaultDailyMargin || rec.Error == "" {
		t.Errorf("unknown record: margin %v error %q, want the default margin and the config error", rec.Margin, rec.Error)
	}
}

func TestUnsavableUnknownRemovesYesterdaysRoom(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-4 * 24 * time.Hour)
	yesterdayRoom(t, stateDir, now)
	swapAtomicWrite(t, func(string, []byte) error { return errors.New("disk full") })
	_, err := runDaily(context.Background(), now, []ReadingSource{failingSource("endpoint broke")})
	if err == nil {
		t.Fatal("want error")
	}
	if code, line := roomVerdict(now.Add(time.Minute)); code != 2 {
		t.Errorf("room after an unsavable failed run: got %d (%q) want 2", code, line)
	}
}

func TestUnsavableRoomRemovesYesterdaysRoom(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-4 * 24 * time.Hour)
	yesterdayRoom(t, stateDir, now)
	swapAtomicWrite(t, func(string, []byte) error { return errors.New("disk full") })
	good := fixedReading(Reading{Source: "endpoint", WeeklyUsed: 50, WeeklyResetsAt: acceptResetsAt})
	if _, err := runDaily(context.Background(), now, []ReadingSource{good}); err == nil {
		t.Fatal("want save error")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "daily.json")); !os.IsNotExist(err) {
		t.Errorf("daily.json still present after a failed save: %v", err)
	}
}

func swapAtomicWrite(t *testing.T, f func(string, []byte) error) {
	t.Helper()
	old := atomicWrite
	atomicWrite = f
	t.Cleanup(func() { atomicWrite = old })
}

func TestEndpointNegativeUtilizationIsAFailedRead(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-7*24*time.Hour + time.Hour)
	u.setWeekly(-50, acceptResetsAt)
	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err == nil {
		t.Error("runDaily: want error for utilization -50")
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown {
		t.Errorf("verdict: got %q want unknown", rec.Verdict)
	}
}

func TestWeeklyUsedOutOfRangeIsAFailedRead(t *testing.T) {
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	for name, used := range map[string]float64{"negative": -0.1, "over 100": 100.1, "NaN": math.NaN(), "+Inf": math.Inf(1)} {
		for _, source := range []string{"endpoint", "probe"} {
			t.Run(name+" "+source, func(t *testing.T) {
				_, stateDir := dailyFixture(t)
				bad := fixedReading(Reading{Source: source, WeeklyUsed: used, WeeklyResetsAt: acceptResetsAt})
				if _, err := runDaily(context.Background(), now, []ReadingSource{bad}); err == nil {
					t.Error("runDaily: want error")
				}
				if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown {
					t.Errorf("verdict: got %q want unknown", rec.Verdict)
				}
			})
		}
	}
	for _, used := range []float64{0, 100} {
		_, stateDir := dailyFixture(t)
		ok := fixedReading(Reading{Source: "probe", WeeklyUsed: used, WeeklyResetsAt: acceptResetsAt})
		if _, err := runDaily(context.Background(), now, []ReadingSource{ok}); err != nil {
			t.Errorf("used %v: %v", used, err)
		}
		if rec := readDailyJSON(t, stateDir); rec.Verdict == VerdictUnknown {
			t.Errorf("used %v: verdict unknown", used)
		}
	}
}

func TestRoomFutureTakenAtIsUnknown(t *testing.T) {
	now := acceptResetsAt.Add(-3 * 24 * time.Hour)
	for name, c := range map[string]struct {
		ahead time.Duration
		code  int
	}{
		"48h ahead":  {48 * time.Hour, 2},
		"6m ahead":   {6 * time.Minute, 2},
		"4m ahead":   {4 * time.Minute, 0},
		"1h earlier": {-time.Hour, 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, stateDir := dailyFixture(t)
			writeDailyJSON(t, stateDir, DailyRecord{
				TakenAt: now.Add(c.ahead), Source: "endpoint", Verdict: VerdictRoom,
				WeeklyUsed: 10, WeeklyResetsAt: acceptResetsAt, Pace: 40, Headroom: 30, Margin: 10,
			})
			if code, line := roomVerdict(now); code != c.code {
				t.Errorf("got %d (%q) want %d", code, line, c.code)
			}
		})
	}
}

func TestDailyResetBoundIsSevenDaysAndAnHour(t *testing.T) {
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	for ahead, wantErr := range map[time.Duration]bool{
		7*24*time.Hour + 30*time.Minute: false,
		7*24*time.Hour + 90*time.Minute: true,
	} {
		err := checkReading(Reading{Source: "endpoint", WeeklyUsed: 1, WeeklyResetsAt: now.Add(ahead)}, now)
		if (err != nil) != wantErr {
			t.Errorf("reset %v ahead: err %v, want error %v", ahead, err, wantErr)
		}
	}
}
