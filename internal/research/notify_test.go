package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTellCoordinatorRunsFromProjectRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	os.Mkdir(root, 0o700)
	log := filepath.Join(dir, "log")
	bin := filepath.Join(dir, "spore")
	os.WriteFile(bin, []byte("#!/bin/sh\npwd > "+log+"\nprintf '%s\\n' \"$@\" >> "+log+"\n"), 0o700)
	if err := TellCoordinator(bin, root)("research demo stopped"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	b, _ := os.ReadFile(log)
	if want := root + "\ntask\ntell\ncoordinator\nresearch demo stopped\n"; string(b) != want {
		t.Errorf("tell ran as\n%q\nwant\n%q", b, want)
	}
}

func TestTellCoordinatorReportsFailure(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "spore")
	os.WriteFile(bin, []byte("#!/bin/sh\necho no coordinator >&2\nexit 3\n"), 0o700)
	err := TellCoordinator(bin, t.TempDir())("x")
	if err == nil || !strings.Contains(err.Error(), "no coordinator") {
		t.Errorf("notify = %v, want the tell failure with its output", err)
	}
}
