// Package acceptpending keeps an acceptance test red-but-skipped until
// the phase that implements it lands. See docs/todo/autoresearch.md.
package acceptpending

import (
	"os"
	"testing"
)

// Skip marks an acceptance test whose phase has not landed yet.
func Skip(t testing.TB, phase string) {
	t.Helper()
	if os.Getenv("SPORE_ACCEPTANCE_STRICT") == "1" {
		return
	}
	t.Skipf("acceptance pending phase %s", phase)
}
