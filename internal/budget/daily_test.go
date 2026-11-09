package budget

import (
	"math"
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
