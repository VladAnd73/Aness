package budget

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	if code, line := RoomCode(now.Add(time.Minute)); code != 2 {
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
	if code, line := RoomCode(now.Add(time.Minute)); code != 2 {
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
			if code, line := RoomCode(now); code != c.code {
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

func captureDailyStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := dailyStderr
	dailyStderr = &buf
	t.Cleanup(func() { dailyStderr = old })
	return &buf
}

func TestHistoryFailureAfterSaveWarnsAndSucceeds(t *testing.T) {
	u, stateDir := dailyFixture(t)
	stderr := captureDailyStderr(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	if err := os.MkdirAll(filepath.Join(stateDir, "daily-history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	u.setWeekly(15, acceptResetsAt)
	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Errorf("runDaily: got %v, want success: the fresh verdict is live", err)
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictRoom {
		t.Errorf("verdict: got %q want room", rec.Verdict)
	}
	if !strings.Contains(stderr.String(), "daily-history.jsonl") {
		t.Errorf("stderr: got %q, want a warning naming the history file", stderr.String())
	}
}

func TestFallbackRecordKeepsEarlierSourceErrors(t *testing.T) {
	_, stateDir := dailyFixture(t)
	stderr := captureDailyStderr(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	good := fixedReading(Reading{Source: "probe", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt})
	if _, err := runDaily(context.Background(), now, []ReadingSource{failingSource("endpoint: HTTP 503"), good}); err != nil {
		t.Fatalf("runDaily: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Source != "probe" || len(rec.Errors) != 1 || rec.Errors[0] != "endpoint: HTTP 503" {
		t.Errorf("record: source %q errors %q, want probe with the endpoint error", rec.Source, rec.Errors)
	}
	if !strings.Contains(stderr.String(), "endpoint: HTTP 503") {
		t.Errorf("stderr: got %q, want the endpoint error", stderr.String())
	}
}

func TestFailedDayHistoryHasNoZeroNumbers(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	_, _ = runDaily(context.Background(), now, []ReadingSource{failingSource("endpoint broke")})
	hist, err := os.ReadFile(filepath.Join(stateDir, "daily-history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"weekly_used", "weekly_resets_at", "pace", "headroom", "five_hour_used", "five_hour_resets_at"} {
		if strings.Contains(string(hist), `"`+key+`"`) {
			t.Errorf("failed day carries %s: %s", key, hist)
		}
	}
	for _, key := range []string{`"verdict":"unknown"`, `"taken_at"`, `"margin":10`, `"error":"endpoint broke"`} {
		if !strings.Contains(string(hist), key) {
			t.Errorf("failed day lacks %s: %s", key, hist)
		}
	}
}

func TestConcurrentDailyRunsDoNotOverlap(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	var mu sync.Mutex
	inside, overlapped := 0, false
	slow := func(context.Context, time.Time) (Reading, error) {
		mu.Lock()
		inside++
		overlapped = overlapped || inside > 1
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		inside--
		mu.Unlock()
		return Reading{Source: "probe", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt}, nil
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := runDaily(context.Background(), now, []ReadingSource{slow}); err != nil {
				t.Errorf("runDaily: %v", err)
			}
		}()
	}
	wg.Wait()
	if overlapped {
		t.Error("two daily runs took a reading at the same time")
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictRoom {
		t.Errorf("verdict: got %q want room", rec.Verdict)
	}
}

func TestDailySaveDoesNotShareATempName(t *testing.T) {
	_, stateDir := dailyFixture(t)
	if err := os.MkdirAll(filepath.Join(stateDir, "daily.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	good := fixedReading(Reading{Source: "endpoint", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt})
	if _, err := runDaily(context.Background(), acceptResetsAt.Add(-5*24*time.Hour), []ReadingSource{good}); err != nil {
		t.Fatalf("runDaily with a foreign daily.json.tmp present: %v", err)
	}
	fi, err := os.Stat(filepath.Join(stateDir, "daily.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("daily.json: %v %v", fi, err)
	}
	if left, _ := filepath.Glob(filepath.Join(stateDir, "daily.json.*")); len(left) != 1 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestCancelledWhileWaitingForLockRecordsUnknown(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-4 * 24 * time.Hour)
	yesterdayRoom(t, stateDir, now)
	held, err := os.OpenFile(filepath.Join(stateDir, "daily.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := syscall.Flock(int(held.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := runDaily(ctx, now, []ReadingSource{failingSource("never reached")}); err == nil {
		t.Fatal("want error")
	}
	if code, line := RoomCode(now.Add(time.Minute)); code != 2 {
		t.Errorf("room after a run cancelled at the lock: got %d (%q) want 2", code, line)
	}
}
