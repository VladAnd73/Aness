package research

import (
	"context"
	"time"
)

type Deps struct {
	Editor     Editor
	Judge      Judge
	Room       func() (code int, line string)
	WeeklyUsed func(ctx context.Context) (float64, error)
	Notify     func(msg string) error
	Now        func() time.Time
	LockPath   string
}

type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return "" }

type NightSummary struct {
	Night, Tries, Kept   int
	StopReason           string
	StartScore, EndScore float64
}

func RunNight(ctx context.Context, s Store, name string, d Deps) (NightSummary, error) {
	return NightSummary{}, nil
}
