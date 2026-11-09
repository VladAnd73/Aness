package budget

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var acceptResetsAt = time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)

type usageServer struct {
	mu   sync.Mutex
	code int
	body string
}

func (u *usageServer) set(code int, body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.code, u.body = code, body
}

func (u *usageServer) setWeekly(used float64, resetsAt time.Time) {
	u.set(http.StatusOK, fmt.Sprintf(
		`{"five_hour":{"utilization":3,"resets_at":%q},"seven_day":{"utilization":%g,"resets_at":%q}}`,
		resetsAt.Add(-7*24*time.Hour+5*time.Hour).Format("2006-01-02T15:04:05.000000+00:00"),
		used,
		resetsAt.Format("2006-01-02T15:04:05.000000+00:00"),
	))
}

func dailyFixture(t *testing.T) (*usageServer, string) {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	t.Setenv("AGENT_BUDGET_STATE_DIR", stateDir)
	t.Setenv("SPORE_BUDGET_CONFIG", filepath.Join(dir, "budget.toml"))
	creds := filepath.Join(dir, ".credentials.json")
	writeCredentials(t, creds, "at-daily", "", time.Now().Add(time.Hour).UnixMilli())
	t.Setenv("AGENT_BUDGET_CREDS", creds)

	u := &usageServer{code: http.StatusInternalServerError, body: "unset"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		code, body := u.code, u.body
		u.mu.Unlock()
		w.WriteHeader(code)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	swapURLs(t, srv.URL, srv.URL+"/token-unused")
	return u, stateDir
}

func readDailyJSON(t *testing.T, stateDir string) DailyRecord {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stateDir, "daily.json"))
	if err != nil {
		t.Fatalf("read daily.json: %v", err)
	}
	var rec DailyRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("parse daily.json: %v", err)
	}
	return rec
}

func writeDailyJSON(t *testing.T, stateDir string, rec DailyRecord) {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daily.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func near(got, want float64) bool { return math.Abs(got-want) <= 0.05 }

func fixedReading(r Reading) ReadingSource {
	return func(context.Context, time.Time) (Reading, error) { return r, nil }
}

func failingSource(msg string) ReadingSource {
	return func(context.Context, time.Time) (Reading, error) { return Reading{}, errors.New(msg) }
}

func TestAcceptance1EndpointRoom(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	u.setWeekly(15, acceptResetsAt)

	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Fatalf("runDaily: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Verdict != VerdictRoom {
		t.Errorf("verdict: got %q want room", rec.Verdict)
	}
	if math.Abs(rec.Headroom-13.57) > 0.01 {
		t.Errorf("headroom: got %v want ~13.57", rec.Headroom)
	}
	if rec.Source != "endpoint" {
		t.Errorf("source: got %q want endpoint", rec.Source)
	}
	if code, line := roomVerdict(now); code != 0 {
		t.Errorf("room code: got %d (%q) want 0", code, line)
	}
}

func TestAcceptance2EndpointNoRoom(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	u.setWeekly(25, acceptResetsAt)

	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Fatalf("runDaily: %v", err)
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictNoRoom {
		t.Errorf("verdict: got %q want no-room", rec.Verdict)
	}
	if code, line := roomVerdict(now); code != 1 {
		t.Errorf("room code: got %d (%q) want 1", code, line)
	}
}

func TestAcceptance3ProbeFallback(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	u.set(http.StatusInternalServerError, "boom")
	probeSrc := fixedReading(Reading{Source: "probe", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt})

	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading, probeSrc}); err != nil {
		t.Fatalf("runDaily: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Verdict != VerdictRoom {
		t.Errorf("verdict: got %q want room", rec.Verdict)
	}
	if rec.Source != "probe" {
		t.Errorf("source: got %q want probe", rec.Source)
	}
}

func TestAcceptance4BothFail(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	u.set(http.StatusInternalServerError, "boom")

	_, err := runDaily(context.Background(), now, []ReadingSource{endpointReading, failingSource("probe broke")})
	if err == nil {
		t.Fatal("runDaily: want error when every source fails")
	}
	if rec := readDailyJSON(t, stateDir); rec.Verdict != VerdictUnknown {
		t.Errorf("verdict: got %q want unknown", rec.Verdict)
	}
	if code, line := roomVerdict(now); code != 2 {
		t.Errorf("room code: got %d (%q) want 2", code, line)
	}
}

func TestAcceptance5Stale(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-3 * 24 * time.Hour)
	writeDailyJSON(t, stateDir, DailyRecord{
		TakenAt: now.Add(-27 * time.Hour), Source: "endpoint", Verdict: VerdictRoom,
		WeeklyUsed: 10, WeeklyResetsAt: acceptResetsAt, Pace: 40, Headroom: 30, Margin: 10,
	})
	if code, line := roomVerdict(now); code != 2 {
		t.Errorf("room code: got %d (%q) want 2", code, line)
	}
}

func TestAcceptance5bResetPassed(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(time.Hour)
	writeDailyJSON(t, stateDir, DailyRecord{
		TakenAt: now.Add(-12 * time.Hour), Source: "endpoint", Verdict: VerdictRoom,
		WeeklyUsed: 50, WeeklyResetsAt: acceptResetsAt, Pace: 93, Headroom: 43, Margin: 10,
	})
	if code, line := roomVerdict(now); code != 1 {
		t.Errorf("room code: got %d (%q) want 1", code, line)
	}
}

func TestAcceptance6Margin(t *testing.T) {
	u, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	u.setWeekly(22, acceptResetsAt)

	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Fatalf("runDaily without config: %v", err)
	}
	rec := readDailyJSON(t, stateDir)
	if rec.Margin != 10 {
		t.Errorf("default margin: got %v want 10", rec.Margin)
	}
	if rec.Verdict != VerdictNoRoom {
		t.Errorf("verdict at margin 10: got %q want no-room", rec.Verdict)
	}

	if err := os.WriteFile(os.Getenv("SPORE_BUDGET_CONFIG"), []byte("margin = 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runDaily(context.Background(), now, []ReadingSource{endpointReading}); err != nil {
		t.Fatalf("runDaily with margin 5: %v", err)
	}
	rec = readDailyJSON(t, stateDir)
	if rec.Margin != 5 {
		t.Errorf("configured margin: got %v want 5", rec.Margin)
	}
	if !near(rec.Pace, 28.6) {
		t.Errorf("pace: got %v want ~28.6", rec.Pace)
	}
	if rec.Verdict != VerdictRoom {
		t.Errorf("verdict at margin 5: got %q want room", rec.Verdict)
	}
}

type recordedCall struct {
	env  []string
	name string
	args []string
}

type callRecorder struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (c *callRecorder) runner(onCall func(name string, args []string)) commandRunner {
	return func(_ context.Context, env []string, name string, args ...string) error {
		c.mu.Lock()
		c.calls = append(c.calls, recordedCall{env: append([]string(nil), env...), name: name, args: append([]string(nil), args...)})
		c.mu.Unlock()
		if onCall != nil {
			onCall(name, args)
		}
		return nil
	}
}

func (c *callRecorder) snapshot() []recordedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedCall(nil), c.calls...)
}

func TestAcceptance7ProbeIsolation(t *testing.T) {
	dir := t.TempDir()
	rec := &callRecorder{}
	p := newProbe(dir)
	p.run = rec.runner(nil)
	p.environ = func() []string {
		return []string{"SPORE_TASK_SLUG=coordinator", "SPORE_PROJECT_ROOT=/x", "HOME=/h", "PATH=/bin"}
	}
	p.timeout = 50 * time.Millisecond
	p.poll = 5 * time.Millisecond

	_, _ = p.reading(context.Background(), time.Now())

	calls := rec.snapshot()
	sawNewSession := false
	for _, c := range calls {
		for _, kv := range c.env {
			if strings.HasPrefix(kv, "SPORE_") {
				t.Errorf("call %s %v carries %s", c.name, c.args, kv)
			}
		}
		if c.name != "tmux" {
			continue
		}
		if len(c.args) < 2 || c.args[0] != "-L" || c.args[1] != "spore-budget-probe" {
			t.Errorf("tmux call not on the dedicated socket: %v", c.args)
		}
		for _, a := range c.args {
			if a == "new-session" {
				sawNewSession = true
			}
		}
	}
	if !sawNewSession {
		t.Errorf("probe never started a tmux session; calls: %+v", calls)
	}
}

func TestAcceptance8ThreeDaysEndToEnd(t *testing.T) {
	u, stateDir := dailyFixture(t)
	days := []struct {
		now      time.Time
		used     float64
		pace     float64
		headroom float64
		verdict  string
		code     int
	}{
		{acceptResetsAt.Add(-6 * 24 * time.Hour), 5, 14.3, 9.3, VerdictNoRoom, 1},
		{acceptResetsAt.Add(-5 * 24 * time.Hour), 20, 28.6, 8.6, VerdictNoRoom, 1},
		{acceptResetsAt.Add(-4 * 24 * time.Hour), 25, 42.9, 17.9, VerdictRoom, 0},
	}
	for i, d := range days {
		u.setWeekly(d.used, acceptResetsAt)
		if _, err := runDaily(context.Background(), d.now, []ReadingSource{endpointReading}); err != nil {
			t.Fatalf("day %d runDaily: %v", i+1, err)
		}
		if code, line := roomVerdict(d.now.Add(time.Hour)); code != d.code {
			t.Errorf("day %d room code: got %d (%q) want %d", i+1, code, line, d.code)
		}
	}

	f, err := os.Open(filepath.Join(stateDir, "daily-history.jsonl"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer f.Close()
	var lines []DailyRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec DailyRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("history line %q: %v", sc.Text(), err)
		}
		lines = append(lines, rec)
	}
	if len(lines) != len(days) {
		t.Fatalf("history lines: got %d want %d", len(lines), len(days))
	}
	for i, d := range days {
		got := lines[i]
		if !near(got.Pace, d.pace) || !near(got.Headroom, d.headroom) || got.Verdict != d.verdict {
			t.Errorf("day %d: got pace %.2f headroom %.2f verdict %q, want %.1f %.1f %q",
				i+1, got.Pace, got.Headroom, got.Verdict, d.pace, d.headroom, d.verdict)
		}
		if !got.TakenAt.Equal(d.now) {
			t.Errorf("day %d taken_at: got %v want %v", i+1, got.TakenAt, d.now)
		}
	}
}
