// Package research runs an autoresearch campaign: an overnight loop
// that edits one target file, scores each edit with a locked judge,
// and keeps only confirmed improvements. See docs/todo/autoresearch.md.
package research

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/versality/spore/internal/evalharness"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusReady    Status = "ready"
	StatusOn       Status = "on"
	StatusOff      Status = "off"
	StatusStopped  Status = "stopped"
	StatusAccepted Status = "accepted"
)

type Config struct {
	Name, Target, SkillName, Objective, Model, GoalFile                        string
	AgentBinary                                                                string
	JudgeArgv                                                                  []string
	MaxTriesPerNight, MaxNights, StopAfterNightsWithoutGain, Parallel, Repeats int
	MaxHoursPerNight, QuotaCeilingPoints                                       float64
	RunTimeout                                                                 time.Duration
	Isolation                                                                  string // "bwrap" (default) or "none"
	Status                                                                     Status
	StopReason                                                                 string
	StartBytes, BestBytes                                                      int
	StartScore, BestScore                                                      float64
	BestCommit, StartCommit                                                    string
	MainHash, HoldoutHash, BinaryHash, LiveFolderHash                          string
	Nights, NoGainNights                                                       int
	HoldoutVerdict                                                             string // "", "pass", "overfit"
	QuotaUsed, JudgeCostUSD                                                    float64
}

// Store is the directory holding every campaign, one folder each.
type Store struct{ Root string }

// DefaultStore is $XDG_STATE_HOME/spore/research, falling back to
// $HOME/.local/state/spore/research.
func DefaultStore() (Store, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return Store{}, errors.New("research: neither XDG_STATE_HOME nor HOME set")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return Store{Root: filepath.Join(base, "spore", "research")}, nil
}

func (s Store) Dir(name string) string { return filepath.Join(s.Root, name) }

func (s Store) repo(name string) repo { return repo{dir: filepath.Join(s.Dir(name), "repo")} }

func (cfg Config) targetRel() string { return filepath.Base(cfg.Target) }

func checkName(name string) error {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\ `) || !filepath.IsLocal(name) {
		return fmt.Errorf("research: campaign name %q must be one plain path element", name)
	}
	return nil
}

// Create starts a draft campaign: it copies the target's whole folder
// into a fresh campaign repo and commits it as "start". The live folder
// is only read.
func (s Store) Create(name, target, goalFile, objective string) (Config, error) {
	if err := checkName(name); err != nil {
		return Config{}, err
	}
	if _, err := LookupObjective(objective); err != nil {
		return Config{}, err
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return Config{}, err
	}
	fi, err := os.Lstat(target)
	if err != nil {
		return Config{}, fmt.Errorf("research: target: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return Config{}, fmt.Errorf("research: target %s is not a regular file", target)
	}
	goal, err := os.ReadFile(goalFile)
	if err != nil {
		return Config{}, fmt.Errorf("research: goal: %w", err)
	}
	dir := s.Dir(name)
	if _, err := os.Lstat(dir); err == nil {
		return Config{}, fmt.Errorf("research: campaign %s already exists at %s", name, dir)
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return Config{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return Config{}, fmt.Errorf("research: %w", err)
	}
	liveDir := filepath.Dir(target)
	liveHash, err := HashTree(liveDir)
	if err != nil {
		return Config{}, err
	}
	r := s.repo(name)
	if err := copyFolder(liveDir, r.dir); err != nil {
		return Config{}, fmt.Errorf("research: copy skill folder: %w", err)
	}
	if err := r.init(); err != nil {
		return Config{}, err
	}
	start, err := r.Commit("start")
	if err != nil {
		return Config{}, err
	}
	goalPath := filepath.Join(dir, "goal.md")
	if err := os.WriteFile(goalPath, goal, 0o600); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Name: name, Target: target, SkillName: filepath.Base(liveDir), Objective: objective,
		GoalFile: goalPath, Isolation: "bwrap", Status: StatusDraft,
		MaxTriesPerNight: 20, MaxHoursPerNight: 8, QuotaCeilingPoints: 15, MaxNights: 7,
		StopAfterNightsWithoutGain: 3, Repeats: 3, Parallel: 3, RunTimeout: 5 * time.Minute,
		StartBytes: int(fi.Size()), BestBytes: int(fi.Size()), StartCommit: start, LiveFolderHash: liveHash,
	}
	return cfg, s.Save(cfg)
}

// copyFolder copies regular files and directories, keeping only the
// exec bit of each file's mode. Anything else is refused.
func copyFolder(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if d.Name() == ".git" {
				return fmt.Errorf("%s: a nested git repository cannot be copied", path)
			}
			return os.MkdirAll(out, 0o700)
		case d.Type().IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			mode := os.FileMode(0o600)
			if fi.Mode().Perm()&0o111 != 0 {
				mode = 0o700
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(out, b, mode)
		default:
			return fmt.Errorf("%s: only regular files and directories are copied", path)
		}
	})
}

// ApproveJudge imports the drafted main and holdout sets, pins a copy
// of binary as the judge, and locks all three by hash. An empty
// judgeArgv means `<pinned spore> eval judge` with the campaign's
// flags; a non-empty one is a different judge command used verbatim.
func (s Store) ApproveJudge(name, draftsDir, binary string, judgeArgv []string) (Config, error) {
	cfg, err := s.Load(name)
	if err != nil {
		return cfg, err
	}
	if cfg.Status != StatusDraft && cfg.Status != StatusReady {
		return cfg, fmt.Errorf("research: campaign %s is %s; a judge is approved only on a draft or ready campaign", name, cfg.Status)
	}
	for _, set := range []string{"main", "holdout"} {
		if _, err := evalharness.LoadSet(filepath.Join(draftsDir, set)); err != nil {
			return cfg, fmt.Errorf("research: judge approve: %s set: %w", set, err)
		}
	}
	dir := s.Dir(name)
	judgeDir := filepath.Join(dir, "judge")
	if err := os.RemoveAll(judgeDir); err != nil {
		return cfg, err
	}
	for _, set := range []string{"main", "holdout"} {
		if err := copyFolder(filepath.Join(draftsDir, set), filepath.Join(judgeDir, set)); err != nil {
			return cfg, fmt.Errorf("research: judge approve: %w", err)
		}
	}
	pinned := s.pinnedBinary(name)
	if err := os.MkdirAll(filepath.Dir(pinned), 0o700); err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(binary)
	if err != nil {
		return cfg, fmt.Errorf("research: judge binary: %w", err)
	}
	if err := os.WriteFile(pinned, b, 0o700); err != nil {
		return cfg, err
	}
	if err := os.Chmod(pinned, 0o700); err != nil {
		return cfg, err
	}
	if cfg.MainHash, err = HashTree(filepath.Join(judgeDir, "main")); err != nil {
		return cfg, err
	}
	if cfg.HoldoutHash, err = HashTree(filepath.Join(judgeDir, "holdout")); err != nil {
		return cfg, err
	}
	if cfg.BinaryHash, err = HashFile(pinned); err != nil {
		return cfg, err
	}
	cfg.JudgeArgv = judgeArgv
	if len(judgeArgv) == 0 {
		cfg.JudgeArgv = defaultJudgeArgv(cfg, pinned, judgeDir, s.repo(name).dir)
	}
	cfg.Status = StatusReady
	return cfg, s.Save(cfg)
}

func defaultJudgeArgv(cfg Config, pinned, judgeDir, repoDir string) []string {
	argv := []string{pinned, "eval", "judge", "--scenarios-root", judgeDir, "--frozen", repoDir, "--skill", cfg.SkillName,
		"--repeats", strconv.Itoa(cfg.Repeats), "--parallel", strconv.Itoa(cfg.Parallel),
		"--timeout", cfg.RunTimeout.String(), "--isolation", cfg.Isolation}
	if cfg.Model != "" {
		argv = append(argv, "--model", cfg.Model)
	}
	if cfg.AgentBinary != "" {
		argv = append(argv, "--agent-binary", cfg.AgentBinary)
	}
	return argv
}

// SetOn switches a campaign on (from ready, or from off while nights
// remain) or off (reason "manual").
func (s Store) SetOn(name string, on bool) error {
	cfg, err := s.Load(name)
	if err != nil {
		return err
	}
	switch {
	case on && cfg.Status == StatusReady, on && cfg.Status == StatusOff && cfg.Nights < cfg.MaxNights:
		cfg.Status, cfg.StopReason = StatusOn, ""
		if cfg.NoGainNights >= cfg.StopAfterNightsWithoutGain {
			cfg.NoGainNights = 0
		}
	case on:
		return fmt.Errorf("research: campaign %s is %s (nights %d of %d); it cannot be switched on", name, cfg.Status, cfg.Nights, cfg.MaxNights)
	case cfg.Status == StatusOn:
		cfg.Status, cfg.StopReason = StatusOff, "manual"
	case cfg.Status == StatusOff:
		return nil
	default:
		return fmt.Errorf("research: campaign %s is %s, not on", name, cfg.Status)
	}
	return s.Save(cfg)
}
