package budget

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	dailyStaleAfter  = 26 * time.Hour
	dailyFutureSlack = 5 * time.Minute
)

func roomVerdict(now time.Time) (code int, line string) {
	d, err := stateDir()
	if err != nil {
		return 2, fmt.Sprintf("unknown state dir: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(d, "daily.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return 2, "unknown no daily reading"
	}
	if err != nil {
		return 2, fmt.Sprintf("unknown unreadable daily.json: %v", err)
	}
	var rec DailyRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return 2, fmt.Sprintf("unknown unreadable daily.json: %v", err)
	}
	if rec.TakenAt.IsZero() || now.Sub(rec.TakenAt) > dailyStaleAfter {
		return 2, roomLine(VerdictUnknown, rec, "stale")
	}
	// A reading from the future means the clock moved; its age, and so
	// its staleness, cannot be trusted.
	if rec.TakenAt.Sub(now) > dailyFutureSlack {
		return 2, roomLine(VerdictUnknown, rec, "future")
	}
	switch rec.Verdict {
	case VerdictRoom, VerdictNoRoom:
	default:
		return 2, roomLine(VerdictUnknown, rec, "")
	}
	if !rec.WeeklyResetsAt.After(now) {
		return 1, roomLine(VerdictNoRoom, rec, "week-reset")
	}
	if rec.Verdict == VerdictRoom {
		return 0, roomLine(VerdictRoom, rec, "")
	}
	return 1, roomLine(VerdictNoRoom, rec, "")
}

func Room() int {
	code, line := roomVerdict(time.Now())
	fmt.Println(line)
	return code
}

func roomLine(verdict string, rec DailyRecord, note string) string {
	line := fmt.Sprintf("%s used=%.1f%% pace=%.1f%% headroom=%.1f taken=%s source=%s",
		verdict, rec.WeeklyUsed, rec.Pace, rec.Headroom, rec.TakenAt.UTC().Format(time.RFC3339), rec.Source)
	if rec.Source == "" {
		line = fmt.Sprintf("%s taken=%s", verdict, rec.TakenAt.UTC().Format(time.RFC3339))
	}
	if note != "" {
		line += " note=" + note
	}
	return line
}
