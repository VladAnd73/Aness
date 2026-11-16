package fleet

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/versality/spore/internal/sporetoml"
	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

// WorkersConfig captures the [fleet.workers] section from a project's
// spore.toml, overlaid by the host-side workers.toml. It controls the agent each spawned worker runs when the
// task's frontmatter does not already pin one.
//
// Default is the fallback agent name written when neither rules nor
// ratio yield a pick. Empty Default plus empty Ratio plus empty Rules
// is the kernel default: "claude".
//
// Ratio maps an agent name to a percentage (any positive integer; the
// values are normalised against their sum). When set, Reconcile picks
// the agent whose currently spawned share is furthest below its target
// share, so the active fleet converges on the configured split.
//
// Rules maps a task `complexity:` value to an agent name. When the task
// frontmatter carries a matching complexity, the rule wins over Ratio.
type WorkersConfig struct {
	Default string
	Ratio   map[string]int
	Rules   map[string]string
}

// LoadWorkersConfig reads `[fleet.workers]` from <projectRoot>/spore.toml,
// then layers the host-side overlay (see WorkersOverlayPath) on top. A
// missing file returns a zero WorkersConfig with no error so callers
// can treat absent config as "use kernel defaults".
func LoadWorkersConfig(projectRoot string) (WorkersConfig, error) {
	tomlPath := filepath.Join(projectRoot, "spore.toml")
	cfg, _, err := readWorkersFile(tomlPath)
	if err != nil {
		return WorkersConfig{}, err
	}
	path, err := WorkersOverlayPath(projectRoot)
	if err != nil {
		return cfg, nil
	}
	ov, seen, err := readWorkersFile(path)
	if err != nil {
		return WorkersConfig{}, err
	}
	return applyWorkersOverlay(cfg, ov, seen), nil
}

// WorkersOverlayPath is the host-side `[fleet.workers]` file
// `spore fleet worker-agent` writes: <StateDirForProject>/workers.toml.
// It lives outside the project tree so a consumer without spore.toml
// never gets one.
func WorkersOverlayPath(projectRoot string) (string, error) {
	dir, err := task.StateDirForProject(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workers.toml"), nil
}

func readWorkersFile(path string) (WorkersConfig, workersSeen, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkersConfig{}, workersSeen{}, nil
		}
		return WorkersConfig{}, workersSeen{}, fmt.Errorf("workers: read %s: %w", path, err)
	}
	cfg, seen, err := parseWorkersSections(string(b))
	if err != nil {
		return WorkersConfig{}, workersSeen{}, fmt.Errorf("workers: parse %s: %w", path, err)
	}
	return cfg, seen, nil
}

// workersSeen records which overlay keys and tables were present, so
// an explicitly empty ratio or rules table still replaces the base.
type workersSeen struct {
	Default, Ratio, Rules bool
}

// applyWorkersOverlay lets every key or table the overlay sets win:
// default replaces, and a ratio or rules table replaces the whole map.
func applyWorkersOverlay(cfg, ov WorkersConfig, seen workersSeen) WorkersConfig {
	if seen.Default {
		cfg.Default = ov.Default
	}
	if seen.Ratio {
		cfg.Ratio = ov.Ratio
	}
	if seen.Rules {
		cfg.Rules = ov.Rules
	}
	return cfg
}

// parseWorkersTOML reads the [fleet.workers], [fleet.workers.ratio],
// and [fleet.workers.rules] sections of the same tiny TOML subset the
// rest of the kernel parses: bare or quoted scalars, `# comment` lines,
// blank lines. Anything outside those sections is ignored. Malformed
// entries inside the sections are an error.
func parseWorkersTOML(content string) (WorkersConfig, error) {
	cfg, _, err := parseWorkersSections(content)
	return cfg, err
}

func parseWorkersSections(content string) (WorkersConfig, workersSeen, error) {
	var cfg WorkersConfig
	var seen workersSeen
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(content))
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(sporetoml.StripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			switch section {
			case "fleet.workers.ratio":
				seen.Ratio = true
			case "fleet.workers.rules":
				seen.Rules = true
			}
			continue
		}
		switch section {
		case "fleet.workers":
			eq := strings.IndexByte(line, '=')
			if eq <= 0 {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: malformed entry %q", lineNum, line)
			}
			key := strings.TrimSpace(line[:eq])
			val := sporetoml.StripQuotes(strings.TrimSpace(line[eq+1:]))
			if key != "default" {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: unknown key %q in [fleet.workers]", lineNum, key)
			}
			cfg.Default = val
			seen.Default = true
		case "fleet.workers.ratio":
			eq := strings.IndexByte(line, '=')
			if eq <= 0 {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: malformed entry %q", lineNum, line)
			}
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			n, err := strconv.Atoi(val)
			if err != nil {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: ratio %q: want integer, got %q", lineNum, key, val)
			}
			if n < 0 {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: ratio %q must be >= 0, got %d", lineNum, key, n)
			}
			if cfg.Ratio == nil {
				cfg.Ratio = map[string]int{}
			}
			cfg.Ratio[key] = n
		case "fleet.workers.rules":
			eq := strings.IndexByte(line, '=')
			if eq <= 0 {
				return WorkersConfig{}, workersSeen{}, fmt.Errorf("line %d: malformed entry %q", lineNum, line)
			}
			key := strings.TrimSpace(line[:eq])
			val := sporetoml.StripQuotes(strings.TrimSpace(line[eq+1:]))
			if cfg.Rules == nil {
				cfg.Rules = map[string]string{}
			}
			cfg.Rules[key] = val
		}
	}
	if err := scanner.Err(); err != nil {
		return WorkersConfig{}, workersSeen{}, err
	}
	return cfg, seen, nil
}

// DefaultWorkerAgent is the fallback agent name when no spore.toml
// pins one. Matches workerAgentCommand's "claude" interpretation.
const DefaultWorkerAgent = "claude"

// SelectAgent returns the agent name to assign to a worker for the
// given task, honouring this precedence:
//
//  1. Explicit `agent:` already set in the task frontmatter.
//  2. A rule keyed on the task's `complexity:` extra.
//  3. Ratio balancing against the running counts: pick the agent whose
//     current share is furthest below its target share.
//  4. cfg.Default, or DefaultWorkerAgent when Default is empty.
//
// counts is the agent-to-spawned-count map for the current pass; it
// must include workers about to spawn so a single Reconcile pass picks
// agents that actually approach the configured ratio.
func SelectAgent(meta frontmatter.Meta, cfg WorkersConfig, counts map[string]int) string {
	if meta.Agent != "" {
		return meta.Agent
	}
	if cfg.Rules != nil {
		if c := meta.Extra["complexity"]; c != "" {
			if a, ok := cfg.Rules[c]; ok && a != "" {
				return a
			}
		}
	}
	if a := selectByRatio(cfg.Ratio, counts); a != "" {
		return a
	}
	if cfg.Default != "" {
		return cfg.Default
	}
	return DefaultWorkerAgent
}

// selectByRatio picks the agent with the largest target-vs-actual
// share deficit. Empty ratio (or all-zero entries) returns "" so the
// caller falls through to the default. Ties resolve in alphabetic
// agent-name order so successive picks are reproducible.
func selectByRatio(ratio map[string]int, counts map[string]int) string {
	if len(ratio) == 0 {
		return ""
	}
	targetTotal := 0
	for _, p := range ratio {
		targetTotal += p
	}
	if targetTotal == 0 {
		return ""
	}
	currTotal := 0
	for k := range ratio {
		currTotal += counts[k]
	}

	keys := make([]string, 0, len(ratio))
	for k := range ratio {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	best := ""
	bestDeficit := -1.0
	for _, k := range keys {
		targetShare := float64(ratio[k]) / float64(targetTotal)
		currShare := 0.0
		if currTotal > 0 {
			currShare = float64(counts[k]) / float64(currTotal)
		}
		deficit := targetShare - currShare
		if deficit > bestDeficit {
			best = k
			bestDeficit = deficit
		}
	}
	return best
}

// Sources EffectiveWorkerDefault reports.
const (
	WorkerDefaultFromOverlay   = "overlay"
	WorkerDefaultFromSporeTOML = "spore.toml"
	WorkerDefaultFromKernel    = "kernel default"
)

// EffectiveWorkerDefault returns the `[fleet.workers] default` agent
// and where it came from: the host overlay, spore.toml, or the kernel
// default.
func EffectiveWorkerDefault(projectRoot string) (agent, source string, err error) {
	path, err := WorkersOverlayPath(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("workers: %w", err)
	}
	ov, seen, err := readWorkersFile(path)
	if err != nil {
		return "", "", err
	}
	if seen.Default && ov.Default != "" {
		return ov.Default, WorkerDefaultFromOverlay, nil
	}
	base, _, err := readWorkersFile(filepath.Join(projectRoot, "spore.toml"))
	if err != nil {
		return "", "", err
	}
	if base.Default != "" {
		return base.Default, WorkerDefaultFromSporeTOML, nil
	}
	return DefaultWorkerAgent, WorkerDefaultFromKernel, nil
}

// SetWorkerDefault writes `default = agent` into the host overlay, or
// deletes the key when agent is "" and removes the file once nothing
// is left in it. spore.toml is never written. Returns the overlay path.
func SetWorkerDefault(projectRoot, agent string) (string, error) {
	path, err := WorkersOverlayPath(projectRoot)
	if err != nil {
		return "", fmt.Errorf("workers: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("workers: read %s: %w", path, err)
	}
	if agent == "" {
		if err != nil {
			return path, nil
		}
		content := sporetoml.DeleteKey(string(b), "fleet.workers", "default")
		if overlayEmpty(content) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("workers: remove %s: %w", path, err)
			}
			return path, nil
		}
		return path, writeOverlay(path, content)
	}
	content := sporetoml.SetKey(string(b), "fleet.workers", "default", agent)
	if _, _, err := parseWorkersSections(content); err != nil {
		return "", fmt.Errorf("workers: %s: %w", path, err)
	}
	return path, writeOverlay(path, content)
}

func writeOverlay(path, content string) error {
	if err := writePrivateFile(path, content); err != nil {
		return fmt.Errorf("workers: write %s: %w", path, err)
	}
	return nil
}

// overlayEmpty reports whether content holds nothing but blank lines,
// comments, and a bare [fleet.workers] header.
func overlayEmpty(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(sporetoml.StripComment(line))
		if line != "" && line != "[fleet.workers]" {
			return false
		}
	}
	return true
}
