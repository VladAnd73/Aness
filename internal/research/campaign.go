// Package research runs an autoresearch campaign: an overnight loop
// that edits one target file, scores each edit with a locked judge,
// and keeps only confirmed improvements. See docs/todo/autoresearch.md.
package research

import (
	"path/filepath"
	"time"
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
	StartBytes                                                                 int
	StartScore, BestScore                                                      float64
	BestCommit, StartCommit                                                    string
	MainHash, HoldoutHash, BinaryHash, LiveFolderHash                          string
	Nights, NoGainNights                                                       int
	HoldoutVerdict                                                             string // "", "pass", "overfit"
}

type Store struct{ Root string }

func DefaultStore() (Store, error) { return Store{}, nil }

func (s Store) Dir(name string) string { return filepath.Join(s.Root, name) }

func (s Store) Create(name, target, goalFile, objective string) (Config, error) {
	return Config{}, nil
}

func (s Store) Load(name string) (Config, error) { return Config{}, nil }

func (s Store) Save(cfg Config) error { return nil }

func (s Store) ApproveJudge(name, draftsDir, binary string, judgeArgv []string) (Config, error) {
	return Config{}, nil
}

func (s Store) SetOn(name string, on bool) error { return nil }
