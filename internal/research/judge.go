package research

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"syscall"
	"time"
)

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

// CommandJudge runs Argv + --candidate P --set S --out R and reads R.
// Its stdout and stderr go to R.log, never into the result.
type CommandJudge struct{ Argv []string }

func (j CommandJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	if len(j.Argv) == 0 {
		return JudgeResult{}, fmt.Errorf("research: judge: empty argv")
	}
	os.Remove(outPath)
	argv := append(append([]string{}, j.Argv...), "--candidate", candidate, "--set", set, "--out", outPath)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Cancel runs before the judge is reaped, so its group id is still
	// its own; killing the group after Wait could hit a reused id.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	log, err := os.OpenFile(outPath+".log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("research: judge: %w", err)
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	runErr := cmd.Run()
	if err := ctx.Err(); err != nil {
		return JudgeResult{}, fmt.Errorf("research: judge: %w", err)
	}
	if runErr != nil {
		return JudgeResult{}, fmt.Errorf("research: judge: %w (see %s.log)", runErr, outPath)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("research: judge: result: %w", err)
	}
	var res JudgeResult
	if err := json.Unmarshal(b, &res); err != nil {
		return JudgeResult{}, fmt.Errorf("research: judge: result %s: %w", outPath, err)
	}
	if math.IsNaN(res.Score) || res.Score < 0 || res.Score > 100 {
		return JudgeResult{}, fmt.Errorf("research: judge: score %v is outside 0..100", res.Score)
	}
	return res, nil
}
