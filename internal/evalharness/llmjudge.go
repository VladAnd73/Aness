package evalharness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const graderPrompt = `You grade one reply from an AI agent against a rubric.

<rubric>
%s
</rubric>

<reply>
%s
</reply>

Judge only the reply against the rubric. End your answer with exactly one line: VERDICT: PASS or VERDICT: FAIL.
`

// gradeLLM asks the grader model whether reply meets the rubric. Only a
// last VERDICT line reading exactly "VERDICT: PASS" passes.
func gradeLLM(ctx context.Context, o JudgeOptions, sb *Sandbox, rubricPath, reply string) (bool, error) {
	rubric, err := os.ReadFile(rubricPath)
	if err != nil {
		return false, err
	}
	wd := filepath.Join(sb.Root, "grader")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		return false, err
	}
	args := []string{"--strict-mcp-config"}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	rctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	res, err := o.Grader.Spawn(rctx, SpawnRequest{
		Prompt:    fmt.Sprintf(graderPrompt, rubric, reply),
		WorkDir:   wd,
		Env:       sb.Env(),
		ExtraArgs: args,
	})
	if err != nil {
		return false, err
	}
	verdict := ""
	for _, line := range strings.Split(res.Stdout, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "VERDICT:") {
			verdict = line
		}
	}
	return verdict == "VERDICT: PASS", nil
}
