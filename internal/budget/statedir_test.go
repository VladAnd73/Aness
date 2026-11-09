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

func TestResolvedStateDirIsAbsoluteFromXDG(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	t.Setenv("AGENT_BUDGET_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "state")
	got, err := ResolvedStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "state", "agent-budget"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEndpointCredentialsFollowClaudeConfigDir(t *testing.T) {
	t.Setenv("AGENT_BUDGET_CREDS", "")
	t.Setenv("HOME", "/h")
	t.Setenv("CLAUDE_CONFIG_DIR", "/c")
	if got := oauthCredsPath(); got != "/c/.credentials.json" {
		t.Errorf("with CLAUDE_CONFIG_DIR: got %q want /c/.credentials.json", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := oauthCredsPath(); got != "/h/.claude/.credentials.json" {
		t.Errorf("without: got %q", got)
	}
	t.Setenv("AGENT_BUDGET_CREDS", "/x.json")
	if got := oauthCredsPath(); got != "/x.json" {
		t.Errorf("override: got %q", got)
	}
}
