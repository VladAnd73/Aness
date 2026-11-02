package fleet

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CoordinatorConfig captures the [coordinator] section from a project's
// spore.toml. Empty fields fall through to env-var overrides and the
// kernel defaults; nothing in this struct overrides an explicit env.
type CoordinatorConfig struct {
	// Driver is the agent provider ("claude", "codex", or a binary
	// path). When set without an explicit SPORE_COORDINATOR_AGENT it
	// supplies the binary the coordinator session execs and seeds
	// SPORE_COORDINATOR_PROVIDER for launcher scripts that dispatch
	// on provider name.
	Driver string

	// Model is the model identifier passed through to the agent
	// (claude --model <m>, codex -m <m>). Surfaced as
	// SPORE_COORDINATOR_MODEL in the session env.
	Model string

	// Brief is the role-file path. Mirrors SPORE_COORDINATOR_ROLE_FILE;
	// relative paths resolve against projectRoot.
	Brief string

	// ExternalSessionPattern is an RE2 regex matched against tmux
	// session names. When set and a live session matches, EnsureCoordinator
	// treats the coordinator role as externally provided and skips the
	// kernel spawn. Use this when an operator-side process owns the
	// coordinator under a non-spore session name (for example a helm-*
	// session running outside the kernel's spore/<project>/coordinator
	// slot). Empty disables the check and the kernel spawns its own.
	ExternalSessionPattern string

	// InboxWatchdogEscalateAfter is how many consecutive reconcile
	// ticks may find this coordinator sitting on an un-acknowledged
	// inbox delivery before the watchdog escalates from a silent
	// requeue to a pane nudge. Zero means use
	// DefaultInboxWatchdogEscalateAfter.
	InboxWatchdogEscalateAfter int

	// SoftCap and HardCap are the context-token caps at which the
	// coordinator token monitor asks for a handoff and then a respawn.
	// Zero means use the tokenmonitor defaults; SPORE_COORDINATOR_SOFT_CAP
	// and SPORE_COORDINATOR_HARD_CAP win over both.
	SoftCap int
	HardCap int
}

// LoadInboxWatchdogEscalateAfter reads
// `[coordinator] inbox_watchdog_escalate_after = N` from
// <projectRoot>/spore.toml, falling back to
// DefaultInboxWatchdogEscalateAfter when the key or the file is absent.
func LoadInboxWatchdogEscalateAfter(projectRoot string) (int, error) {
	cfg, err := LoadCoordinatorConfig(projectRoot)
	if err != nil {
		return DefaultInboxWatchdogEscalateAfter, err
	}
	if cfg.InboxWatchdogEscalateAfter < 1 {
		return DefaultInboxWatchdogEscalateAfter, nil
	}
	return cfg.InboxWatchdogEscalateAfter, nil
}

// LoadCoordinatorConfig reads `[coordinator]` from <projectRoot>/spore.toml.
// A missing file returns a zero CoordinatorConfig with no error so callers
// can treat absent config as "use defaults".
func LoadCoordinatorConfig(projectRoot string) (CoordinatorConfig, error) {
	return loadCoordinatorConfig(projectRoot, os.Stderr)
}

// LoadCoordinatorConfigQuiet is LoadCoordinatorConfig without the
// warnings. Stop hooks use it: their stderr becomes the agent's
// feedback on exit 2, and the fleet reconciler already reports the
// same warnings to the journal.
func LoadCoordinatorConfigQuiet(projectRoot string) (CoordinatorConfig, error) {
	return loadCoordinatorConfig(projectRoot, io.Discard)
}

func loadCoordinatorConfig(projectRoot string, warn io.Writer) (CoordinatorConfig, error) {
	tomlPath := filepath.Join(projectRoot, "spore.toml")
	b, err := os.ReadFile(tomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return CoordinatorConfig{}, nil
		}
		return CoordinatorConfig{}, fmt.Errorf("coordinator: read %s: %w", tomlPath, err)
	}
	cfg, warnings, err := parseCoordinatorTOML(string(b))
	for _, w := range warnings {
		fmt.Fprintf(warn, "coordinator: %s: %s\n", tomlPath, w)
	}
	if err != nil {
		return CoordinatorConfig{}, fmt.Errorf("coordinator: parse %s: %w", tomlPath, err)
	}
	return cfg, nil
}

// parseCoordinatorTOML reads only the [coordinator] section of a tiny
// TOML subset: bare or quoted scalar values, `# comment` lines, and
// blank lines. Anything outside [coordinator] is ignored. A line that
// is not an entry at all is an error, because the file is then not the
// shape this parser understands.
//
// A key this binary does not know, or a value it cannot use, is a
// warning and nothing more. Hosts run several spore versions at once,
// and failing the parse would hand every caller a zero config: a
// single key from a newer release would silently strip driver, model
// and brief from a coordinator an older binary spawns.
func parseCoordinatorTOML(content string) (CoordinatorConfig, []string, error) {
	var cfg CoordinatorConfig
	var warnings []string
	inSection := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(stripTOMLComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.TrimSpace(line[1:len(line)-1]) == "coordinator"
			continue
		}
		if !inSection {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return CoordinatorConfig{}, warnings, fmt.Errorf("line %d: malformed entry %q", lineNum, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := stripTOMLQuotes(strings.TrimSpace(line[eq+1:]))
		switch key {
		case "driver":
			cfg.Driver = val
		case "model":
			cfg.Model = val
		case "brief":
			cfg.Brief = val
		case "external_session_pattern":
			cfg.ExternalSessionPattern = val
		case "inbox_watchdog_escalate_after":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				warnings = append(warnings, fmt.Sprintf(
					"line %d: ignoring inbox_watchdog_escalate_after = %q: want an integer >= 1",
					lineNum, val))
				continue
			}
			cfg.InboxWatchdogEscalateAfter = n
		case "soft_cap", "hard_cap":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				warnings = append(warnings, fmt.Sprintf(
					"line %d: ignoring %s = %q: want an integer >= 1", lineNum, key, val))
				continue
			}
			if key == "soft_cap" {
				cfg.SoftCap = n
			} else {
				cfg.HardCap = n
			}
		default:
			warnings = append(warnings, fmt.Sprintf(
				"line %d: ignoring unknown key %q in [coordinator]", lineNum, key))
		}
	}
	if err := scanner.Err(); err != nil {
		return CoordinatorConfig{}, warnings, err
	}
	return cfg, warnings, nil
}

func stripTOMLComment(line string) string {
	inQuote := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case inQuote != 0:
			if ch == inQuote {
				inQuote = 0
			}
		case ch == '"' || ch == '\'':
			inQuote = ch
		case ch == '#':
			return line[:i]
		}
	}
	return line
}

func stripTOMLQuotes(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
