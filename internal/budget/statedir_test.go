package budget

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolvedStateDirIsAbsolute(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	t.Setenv("AGENT_BUDGET_STATE_DIR", "rel/state")
	got, err := ResolvedStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "rel", "state"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRoomNamesTheDirItRead(t *testing.T) {
	_, stateDir := dailyFixture(t)
	code, line := roomVerdict(time.Now())
	if code != 2 || !strings.Contains(line, stateDir) {
		t.Errorf("got %d %q, want 2 naming %s", code, line, stateDir)
	}
}
