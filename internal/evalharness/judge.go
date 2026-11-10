package evalharness

import (
	"context"
	"time"
)

type JudgeOptions struct {
	SetDir, Candidate, FrozenDir, SkillName, Model, AgentBinary, Isolation, SandboxParent string
	Repeats, Parallel                                                                     int
	Timeout                                                                               time.Duration
	Spawner                                                                               AgentSpawner
}

// JudgeReport carries the same JSON fields as research.JudgeResult.
// evalharness must not import research: research imports evalharness.
type JudgeReport struct {
	Score        float64          `json:"score"`
	Runs         int              `json:"runs"`
	Passed       int              `json:"passed"`
	Crashes      int              `json:"crashes"`
	CostUSD      float64          `json:"cost_usd"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	Scenarios    []ScenarioReport `json:"scenarios"`
}

type ScenarioReport struct {
	Name string      `json:"name"`
	Runs []RunReport `json:"runs"`
}

type RunReport struct {
	Pass           bool   `json:"pass"`
	Crashed        bool   `json:"crashed"`
	Reason         string `json:"reason"`
	TranscriptPath string `json:"transcript_path"`
	CallsLog       string `json:"calls_log"`
	SandboxRoot    string `json:"sandbox_root"`
}

func RunJudge(ctx context.Context, o JudgeOptions) (JudgeReport, error) { return JudgeReport{}, nil }

// ProbeIsolated runs argv under the same isolation RunJudge gives the
// agent under test and returns its combined output.
func ProbeIsolated(ctx context.Context, o JudgeOptions, argv []string) (string, error) {
	return "", nil
}
