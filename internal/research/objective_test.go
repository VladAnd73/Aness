package research

import (
	"strings"
	"testing"
)

func TestShrinkHoldingScore(t *testing.T) {
	obj, err := LookupObjective("shrink-holding-score")
	if err != nil || obj == nil {
		t.Fatalf("LookupObjective: %v, %v", obj, err)
	}
	start := Measured{Score: 60, Bytes: 100}
	best := Measured{Score: 70, Bytes: 100}
	for _, c := range []struct {
		name string
		cand Measured
		want bool
	}{
		{"shorter, equal to start", Measured{Score: 60, Bytes: 90}, true},
		{"shorter, above start", Measured{Score: 90, Bytes: 90}, true},
		{"shorter, below start", Measured{Score: 59.9, Bytes: 90}, false},
		{"same length", Measured{Score: 90, Bytes: 100}, false},
		{"longer", Measured{Score: 90, Bytes: 101}, false},
	} {
		if got := obj.Better(start, best, c.cand); got != c.want {
			t.Errorf("%s: Better = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMaximizeScore(t *testing.T) {
	obj, _ := LookupObjective("maximize-score")
	start, best := Measured{Score: 50, Bytes: 100}, Measured{Score: 60, Bytes: 100}
	if !obj.Better(start, best, Measured{Score: 61, Bytes: 150}) {
		t.Errorf("higher score, longer: want better")
	}
	if obj.Better(start, best, Measured{Score: 59, Bytes: 10}) {
		t.Errorf("lower score, shorter: want not better")
	}
	if obj.Better(start, best, best) {
		t.Errorf("identical: want not better")
	}
	mean := (66.66666666666667 + 66.66666666666666) / 2
	if obj.Better(start, Measured{Score: 66.66666666666667, Bytes: 100}, Measured{Score: mean, Bytes: 100}) {
		t.Errorf("float noise counted as a gain")
	}
}

func TestOverLengthCapBoundary(t *testing.T) {
	for _, c := range []struct {
		start, cand int
		want        bool
	}{{100, 170, false}, {100, 171, true}, {0, 0, false}, {0, 1, true}, {1, 1, false}, {1, 2, true}, {10, 17, false}, {10, 18, true}} {
		if got := OverLengthCap(c.start, c.cand); got != c.want {
			t.Errorf("OverLengthCap(%d, %d) = %v, want %v", c.start, c.cand, got, c.want)
		}
	}
}

func TestLookupUnknownObjective(t *testing.T) {
	_, err := LookupObjective("nope")
	if err == nil || !strings.Contains(err.Error(), "maximize-score") || !strings.Contains(err.Error(), "shrink-holding-score") {
		t.Errorf("LookupObjective(nope) = %v, want an error listing the known names", err)
	}
}
