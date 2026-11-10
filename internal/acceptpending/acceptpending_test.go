package acceptpending

import "testing"

func runSkip(t *testing.T) bool {
	var inner *testing.T
	t.Run("probe", func(st *testing.T) {
		inner = st
		Skip(st, "Z")
	})
	return inner.Skipped()
}

func TestSkipSkipsByDefault(t *testing.T) {
	t.Setenv("SPORE_ACCEPTANCE_STRICT", "")
	if !runSkip(t) {
		t.Error("want the test skipped when SPORE_ACCEPTANCE_STRICT is unset")
	}
}

func TestSkipRunsUnderStrict(t *testing.T) {
	t.Setenv("SPORE_ACCEPTANCE_STRICT", "1")
	if runSkip(t) {
		t.Error("want the test to run when SPORE_ACCEPTANCE_STRICT=1")
	}
}
