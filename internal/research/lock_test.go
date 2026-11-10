package research

import (
	"path/filepath"
	"testing"
)

func TestLockSecondHolderRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "research.lock")
	unlock, _, err := Lock(path, "first")
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	if _, holder, err := Lock(path, "second"); err == nil || holder != "first" {
		t.Errorf("second Lock = holder %q, err %v; want refused naming first", holder, err)
	}
	unlock()
	unlock2, _, err := Lock(path, "second")
	if err != nil {
		t.Errorf("Lock after unlock: %v", err)
	} else {
		unlock2()
	}
}
