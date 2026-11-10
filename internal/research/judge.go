package research

import "context"

type JudgeResult struct {
	Score        float64 `json:"score"`
	Runs         int     `json:"runs"`
	Passed       int     `json:"passed"`
	Crashes      int     `json:"crashes"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

type Judge interface {
	Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error)
}

type CommandJudge struct{ Argv []string }

func (j CommandJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	return JudgeResult{}, nil
}
