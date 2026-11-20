package budget

import (
	"context"
	"time"
)

// WeeklyUsed is the live seven-day utilization in percent, read from
// the same OAuth usage endpoint `spore budget daily` uses. Any failure
// is an error, never a zero reading: callers must fail closed.
func WeeklyUsed(ctx context.Context) (float64, error) {
	r, err := endpointReading(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	return r.WeeklyUsed, nil
}
