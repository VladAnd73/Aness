package research

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
)

type Measured struct {
	Score float64
	Bytes int
}

// Objective decides whether a candidate beats the best so far. A new
// objective registers itself from init; the night loop never names one.
type Objective interface {
	Name() string
	Better(start, best, cand Measured) bool
}

var (
	objectivesMu sync.Mutex
	objectives   = map[string]Objective{}
)

func RegisterObjective(o Objective) {
	objectivesMu.Lock()
	defer objectivesMu.Unlock()
	objectives[o.Name()] = o
}

func LookupObjective(name string) (Objective, error) {
	objectivesMu.Lock()
	defer objectivesMu.Unlock()
	if o, ok := objectives[name]; ok {
		return o, nil
	}
	var names []string
	for n := range objectives {
		names = append(names, n)
	}
	slices.Sort(names)
	return nil, fmt.Errorf("research: unknown objective %q (known: %s)", name, strings.Join(names, ", "))
}

// OverLengthCap reports a candidate more than 70% longer than the start.
func OverLengthCap(startBytes, candBytes int) bool {
	return int64(candBytes)*100 > int64(startBytes)*170
}

// roundScore drops float noise, so the mean of two runs compares equal
// to the same score reached another way.
func roundScore(s float64) float64 { return math.Round(s*1e9) / 1e9 }

type maximizeScore struct{}

func (maximizeScore) Name() string { return "maximize-score" }

func (maximizeScore) Better(start, best, cand Measured) bool {
	c, b := roundScore(cand.Score), roundScore(best.Score)
	return c > b || (c == b && cand.Bytes < best.Bytes)
}

type shrinkHoldingScore struct{}

func (shrinkHoldingScore) Name() string { return "shrink-holding-score" }

func (shrinkHoldingScore) Better(start, best, cand Measured) bool {
	return cand.Bytes < best.Bytes && roundScore(cand.Score) >= roundScore(start.Score)
}

func init() {
	RegisterObjective(maximizeScore{})
	RegisterObjective(shrinkHoldingScore{})
}
