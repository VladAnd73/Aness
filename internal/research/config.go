package research

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/versality/spore/internal/statefile"
)

const configFile = "campaign.toml"

// configFields lists every campaign.toml key in file order. Load and
// Save both walk it, so a field added here round-trips without more code.
func configFields(c *Config) []struct {
	key string
	ptr any
} {
	return []struct {
		key string
		ptr any
	}{
		{"name", &c.Name}, {"status", &c.Status}, {"stop_reason", &c.StopReason},
		{"target", &c.Target}, {"skill_name", &c.SkillName}, {"goal_file", &c.GoalFile},
		{"objective", &c.Objective}, {"model", &c.Model}, {"agent_binary", &c.AgentBinary},
		{"isolation", &c.Isolation}, {"judge_argv", &c.JudgeArgv},
		{"max_tries_per_night", &c.MaxTriesPerNight}, {"max_hours_per_night", &c.MaxHoursPerNight},
		{"quota_ceiling_points", &c.QuotaCeilingPoints}, {"max_nights", &c.MaxNights},
		{"stop_after_nights_without_gain", &c.StopAfterNightsWithoutGain},
		{"repeats", &c.Repeats}, {"parallel", &c.Parallel}, {"run_timeout", &c.RunTimeout},
		{"start_bytes", &c.StartBytes}, {"best_bytes", &c.BestBytes},
		{"start_score", &c.StartScore}, {"best_score", &c.BestScore},
		{"start_commit", &c.StartCommit}, {"best_commit", &c.BestCommit},
		{"main_hash", &c.MainHash}, {"holdout_hash", &c.HoldoutHash},
		{"binary_hash", &c.BinaryHash}, {"live_folder_hash", &c.LiveFolderHash},
		{"nights", &c.Nights}, {"no_gain_nights", &c.NoGainNights},
		{"holdout_verdict", &c.HoldoutVerdict},
		{"quota_used_points", &c.QuotaUsed}, {"judge_cost_usd", &c.JudgeCostUSD},
	}
}

func encodeValue(ptr any) string {
	switch p := ptr.(type) {
	case *string:
		return strconv.Quote(*p)
	case *Status:
		return strconv.Quote(string(*p))
	case *int:
		return strconv.Itoa(*p)
	case *float64:
		return strconv.FormatFloat(*p, 'g', -1, 64)
	case *time.Duration:
		return strconv.Quote(p.String())
	case *[]string:
		q := make([]string, len(*p))
		for i, s := range *p {
			q[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	panic(fmt.Sprintf("research: config: unsupported field type %T", ptr))
}

func decodeValue(ptr any, val string) error {
	switch p := ptr.(type) {
	case *string:
		s, err := strconv.Unquote(val)
		*p = s
		return err
	case *Status:
		s, err := strconv.Unquote(val)
		*p = Status(s)
		return err
	case *int:
		n, err := strconv.Atoi(val)
		*p = n
		return err
	case *float64:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil && (math.IsNaN(f) || math.IsInf(f, 0)) {
			err = errors.New("not a finite number")
		}
		*p = f
		return err
	case *time.Duration:
		s, err := strconv.Unquote(val)
		if err != nil {
			return err
		}
		*p, err = time.ParseDuration(s)
		return err
	case *[]string:
		return decodeList(p, val)
	}
	return fmt.Errorf("unsupported field type %T", ptr)
}

func decodeList(p *[]string, val string) error {
	rest, ok := strings.CutPrefix(val, "[")
	if !ok || !strings.HasSuffix(rest, "]") {
		return errors.New("want a [\"quoted\", \"list\"]")
	}
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "]"))
	var out []string
	for rest != "" {
		q, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return err
		}
		s, _ := strconv.Unquote(q)
		out = append(out, s)
		rest = strings.TrimSpace(rest[len(q):])
		if rest == "" {
			break
		}
		if rest, ok = strings.CutPrefix(rest, ","); !ok {
			return errors.New("list items must be separated by commas")
		}
		rest = strings.TrimSpace(rest)
	}
	*p = out
	return nil
}

func (s Store) configPath(name string) string { return filepath.Join(s.Dir(name), configFile) }

func (s Store) Save(cfg Config) error {
	var b strings.Builder
	for _, f := range configFields(&cfg) {
		fmt.Fprintf(&b, "%s = %s\n", f.key, encodeValue(f.ptr))
	}
	return statefile.WriteFileAtomic(s.configPath(cfg.Name), ".campaign-*.toml", []byte(b.String()))
}

func (s Store) Load(name string) (Config, error) {
	var cfg Config
	path := s.configPath(name)
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("research: campaign %s: %w", name, err)
	}
	fields := map[string]any{}
	for _, f := range configFields(&cfg) {
		fields[f.key] = f.ptr
	}
	for i, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("research: %s line %d: want key = value, got %q", path, i+1, raw)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		ptr, known := fields[key]
		if !known {
			return cfg, fmt.Errorf("research: %s line %d: unknown key %q", path, i+1, key)
		}
		if err := decodeValue(ptr, val); err != nil {
			return cfg, fmt.Errorf("research: %s line %d: %s: %v", path, i+1, key, err)
		}
	}
	return cfg, nil
}
