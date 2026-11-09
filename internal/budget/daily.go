package budget

import (
	"context"
	"errors"
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

func computeVerdict(r Reading, margin float64, now time.Time) DailyRecord {
	return DailyRecord{}
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
