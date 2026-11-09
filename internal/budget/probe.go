package budget

import (
	"context"
	"errors"
	"time"
)

type commandRunner func(ctx context.Context, env []string, name string, args ...string) error

type probe struct {
	run         commandRunner
	dir         string
	captureFile string
	socket      string
	timeout     time.Duration
	poll        time.Duration
	environ     func() []string
}

func newProbe(dir string) probe {
	return probe{}
}

func (p probe) reading(ctx context.Context, now time.Time) (Reading, error) {
	return Reading{}, errors.New("not implemented")
}

func probeReading(ctx context.Context, now time.Time) (Reading, error) {
	return Reading{}, errors.New("probe not available")
}

func probeEnv(base []string) []string {
	return nil
}
