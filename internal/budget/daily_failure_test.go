package budget

import (
	"context"
	"errors"
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
