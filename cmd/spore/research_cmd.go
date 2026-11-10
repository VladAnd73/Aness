package main

import (
	"context"
	"io"

	"github.com/versality/spore/internal/research"
)

// researchDeps carries what a test swaps out of `spore research`.
// SporeBinary is the binary `judge approve` copies into the campaign as
// its pinned judge; empty means the running executable.
type researchDeps struct {
	Editor      research.Editor
	Room        func() (code int, line string)
	WeeklyUsed  func(ctx context.Context) (float64, error)
	Notify      func(msg string) error
	SporeBinary string
}

func researchMain(out, errOut io.Writer, args []string, deps researchDeps) int { return 0 }
