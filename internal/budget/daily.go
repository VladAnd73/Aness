package budget

import (
	"context"
	"errors"
	"math"
	"time"
)

const (
	VerdictRoom    = "room"
	VerdictNoRoom  = "no-room"
	VerdictUnknown = "unknown"
)

type Reading struct {
	Source           string
	WeeklyUsed       float64
	WeeklyResetsAt   time.Time
	FiveHourUsed     float64
	FiveHourResetsAt time.Time
	Extra            map[string]float64
}

type DailyRecord struct {
	TakenAt          time.Time          `json:"taken_at"`
	Source           string             `json:"source"`
	WeeklyUsed       float64            `json:"weekly_used"`
	WeeklyResetsAt   time.Time          `json:"weekly_resets_at"`
	Pace             float64            `json:"pace"`
	Headroom         float64            `json:"headroom"`
	Margin           float64            `json:"margin"`
	Verdict          string             `json:"verdict"`
	FiveHourUsed     float64            `json:"five_hour_used"`
	FiveHourResetsAt time.Time          `json:"five_hour_resets_at"`
	Extra            map[string]float64 `json:"extra,omitempty"`
	Error            string             `json:"error,omitempty"`
}

type ReadingSource func(ctx context.Context, now time.Time) (Reading, error)

const weeklyWindow = 7 * 24 * time.Hour

func computeVerdict(r Reading, margin float64, now time.Time) DailyRecord {
	start := r.WeeklyResetsAt.Add(-weeklyWindow)
	elapsed := float64(now.Sub(start)) / float64(weeklyWindow)
	elapsed = math.Min(math.Max(elapsed, 0), 1)
	pace := elapsed * 100
	headroom := pace - r.WeeklyUsed
	verdict := VerdictNoRoom
	if headroom >= margin {
		verdict = VerdictRoom
	}
	return DailyRecord{
		TakenAt:          now.UTC(),
		Source:           r.Source,
		WeeklyUsed:       r.WeeklyUsed,
		WeeklyResetsAt:   r.WeeklyResetsAt.UTC(),
		Pace:             pace,
		Headroom:         headroom,
		Margin:           margin,
		Verdict:          verdict,
		FiveHourUsed:     r.FiveHourUsed,
		FiveHourResetsAt: r.FiveHourResetsAt.UTC(),
		Extra:            r.Extra,
	}
}

func endpointReading(ctx context.Context, now time.Time) (Reading, error) {
	return Reading{}, errors.New("not implemented")
}

func runDaily(ctx context.Context, now time.Time, sources []ReadingSource) (DailyRecord, error) {
	return DailyRecord{}, errors.New("not implemented")
}

func Daily() error {
	return errors.New("not implemented")
}
