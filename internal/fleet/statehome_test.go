package fleet

import (
	"os"
	"testing"
)

// TestPackageStateHomeIsIsolated guards against tests that resolve
// state paths without their own t.Setenv("XDG_STATE_HOME", ...): such
// a test must land in TestMain's temp dir, never the host state dir.
func TestPackageStateHomeIsIsolated(t *testing.T) {
	if testStateHome == "" {
		t.Fatal("TestMain did not set up an isolated XDG_STATE_HOME")
	}
	if got := os.Getenv("XDG_STATE_HOME"); got != testStateHome {
		t.Fatalf("XDG_STATE_HOME = %q, want TestMain's %q", got, testStateHome)
	}
}
