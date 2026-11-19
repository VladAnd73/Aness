package budget

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoomExitCodes(t *testing.T) {
	now := acceptResetsAt.Add(-3 * 24 * time.Hour)
	base := DailyRecord{
		TakenAt: now.Add(-time.Hour), Source: "endpoint", WeeklyUsed: 15,
		WeeklyResetsAt: acceptResetsAt, Pace: 28.6, Headroom: 13.6, Margin: 10,
	}
	cases := []struct {
		name  string
		write func(t *testing.T, stateDir string)
		code  int
		start string
	}{
		{"room", func(t *testing.T, d string) { r := base; r.Verdict = VerdictRoom; writeDailyJSON(t, d, r) }, 0, "room "},
		{"no-room", func(t *testing.T, d string) { r := base; r.Verdict = VerdictNoRoom; writeDailyJSON(t, d, r) }, 1, "no-room "},
		{"unknown", func(t *testing.T, d string) { r := base; r.Verdict = VerdictUnknown; writeDailyJSON(t, d, r) }, 2, "unknown"},
		{"odd verdict", func(t *testing.T, d string) { r := base; r.Verdict = "maybe"; writeDailyJSON(t, d, r) }, 2, "unknown"},
		{"missing file", func(t *testing.T, d string) {}, 2, "unknown no daily reading in "},
		{"corrupt", func(t *testing.T, d string) {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "daily.json"), []byte(`{"verdict":"room","taken_at":`), 0o600); err != nil {
				t.Fatal(err)
			}
		}, 2, "unknown"},
		{"empty file", func(t *testing.T, d string) {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "daily.json"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, 2, "unknown"},
		{"25h old room", func(t *testing.T, d string) {
			r := base
			r.Verdict = VerdictRoom
			r.TakenAt = now.Add(-25 * time.Hour)
			writeDailyJSON(t, d, r)
		}, 0, "room "},
		{"stale beats reset", func(t *testing.T, d string) {
			r := base
			r.Verdict = VerdictRoom
			r.TakenAt = now.Add(-27 * time.Hour)
			r.WeeklyResetsAt = now.Add(-time.Hour)
			writeDailyJSON(t, d, r)
		}, 2, "unknown"},
		{"reset passed no-room", func(t *testing.T, d string) {
			r := base
			r.Verdict = VerdictRoom
			r.WeeklyResetsAt = now.Add(-time.Minute)
			writeDailyJSON(t, d, r)
		}, 1, "no-room "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stateDir := dailyFixture(t)
			c.write(t, stateDir)
			code, line := RoomCode(now)
			if code != c.code {
				t.Errorf("code: got %d (%q) want %d", code, line, c.code)
			}
			if !strings.HasPrefix(line, c.start) {
				t.Errorf("line: got %q want prefix %q", line, c.start)
			}
		})
	}
}

func TestRoomLineFormat(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := acceptResetsAt.Add(-5 * 24 * time.Hour)
	writeDailyJSON(t, stateDir, computeVerdict(Reading{Source: "endpoint", WeeklyUsed: 15, WeeklyResetsAt: acceptResetsAt}, 10, now))
	_, line := RoomCode(now.Add(time.Hour))
	want := "room used=15.0% pace=28.6% headroom=13.6 margin=10.0 taken=2026-11-11T18:00:00Z source=endpoint"
	if line != want {
		t.Errorf("line:\n got %q\nwant %q", line, want)
	}
}

func TestBudgetRoomCodeMatchesRoom(t *testing.T) {
	_, stateDir := dailyFixture(t)
	now := time.Now()
	writeDailyJSON(t, stateDir, DailyRecord{
		TakenAt: now.Add(-time.Hour), Source: "endpoint", Verdict: VerdictRoom, WeeklyUsed: 15,
		WeeklyResetsAt: now.Add(48 * time.Hour), Pace: 28.6, Headroom: 13.6, Margin: 10,
	})
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	roomCode := Room()
	os.Stdout = stdout
	w.Close()
	printed, _ := io.ReadAll(r)

	code, line := RoomCode(time.Now())
	if code != 0 || code != roomCode {
		t.Errorf("RoomCode = %d, Room = %d; want both 0", code, roomCode)
	}
	if line+"\n" != string(printed) {
		t.Errorf("RoomCode line %q, Room printed %q", line, printed)
	}
}
