package budget

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// stateDir is absolute so a relative AGENT_BUDGET_STATE_DIR or
// XDG_STATE_HOME names the same directory as the one pinned into the
// timer unit.
func stateDir() (string, error) {
	if d := os.Getenv("AGENT_BUDGET_STATE_DIR"); d != "" {
		return filepath.Abs(d)
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Abs(filepath.Join(d, "agent-budget"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "agent-budget"), nil
}

// ResolvedStateDir is what `spore budget state-dir` prints, so the timer
// drop-in and the probe trust step use the directory room reads.
func ResolvedStateDir() (string, error) {
	return stateDir()
}

// TimerEnv renders a systemd drop-in that gives the daily timer what
// this shell resolves: the user manager never sees variables set only
// in a shell, so without it the timer could read another state dir,
// another account or another claude config dir than `room` and a
// manual `daily` do. A variable unset here is unset there too.
func TimerEnv() (string, error) {
	sd, err := stateDir()
	if err != nil {
		return "", err
	}
	cfg, err := filepath.Abs(dailyConfigPath())
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("[Service]\n")
	set := func(key, val string) {
		val = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(val)
		fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", key, val)
	}
	set("AGENT_BUDGET_STATE_DIR", sd)
	set("SPORE_BUDGET_CONFIG", cfg)
	for _, key := range []string{"AGENT_BUDGET_CREDS", "CLAUDE_CONFIG_DIR"} {
		v := os.Getenv(key)
		if v == "" {
			fmt.Fprintf(&b, "UnsetEnvironment=%s\n", key)
			continue
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", err
		}
		set(key, abs)
	}
	return b.String(), nil
}
