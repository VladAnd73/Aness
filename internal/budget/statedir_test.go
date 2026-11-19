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
	code, line := RoomCode(time.Now())
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

func TestTimerEnvPinsWhatTheShellResolves(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	t.Setenv("AGENT_BUDGET_STATE_DIR", "my state")
	t.Setenv("AGENT_BUDGET_CREDS", "/c/100%\"x\".json")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("SPORE_BUDGET_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	got, err := TimerEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[Service]\n",
		`Environment="AGENT_BUDGET_STATE_DIR=` + filepath.Join(base, "my state") + `"` + "\n",
		`Environment="AGENT_BUDGET_CREDS=/c/100%%\"x\".json"` + "\n",
		`Environment="SPORE_BUDGET_CONFIG=/cfg/spore/budget.toml"` + "\n",
		"UnsetEnvironment=CLAUDE_CONFIG_DIR\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("drop-in lacks %q:\n%s", want, got)
		}
	}
}
