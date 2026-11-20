package evalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/versality/spore/internal/isolate"
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
// last VERDICT line reading exactly "VERDICT: PASS" passes. crash names
// a grader run that failed; err is a grader that never ran, which fails
// the whole judge like the agent's own setup errors.
func gradeLLM(ctx context.Context, o JudgeOptions, sb *Sandbox, rubricPath, reply string) (pass bool, crash string, err error) {
	rubric, err := os.ReadFile(rubricPath)
	if err != nil {
		return false, "", err
	}
	out, crash, err := askGrader(ctx, o, sb, fmt.Sprintf(graderPrompt, rubric, reply))
	if err != nil || crash != "" {
		return false, crash, err
	}
	verdict := ""
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "VERDICT:") {
			verdict = line
		}
	}
	return verdict == "VERDICT: PASS", "", nil
}

// askGrader runs the grader model once on prompt and returns its output.
func askGrader(ctx context.Context, o JudgeOptions, sb *Sandbox, prompt string) (out, crash string, err error) {
	wd, home := filepath.Join(sb.Root, "grader"), filepath.Join(sb.Root, "grader-home")
	for _, d := range []string{wd, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", "", err
		}
	}
	// The prompt holds untrusted text from the agent under test: the
	// grader gets no tools to act on it and, unless unsandboxed, the
	// same jail.
	args := []string{"--tools", "", "--strict-mcp-config"}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	req := SpawnRequest{Prompt: prompt, WorkDir: wd, Env: sb.Env(), ExtraArgs: args}
	if o.Isolation != "none" {
		spec := isolate.Spec{Work: wd, Home: home, Credentials: isolate.CredentialsPath(), Env: isolate.BaseEnv(home)}
		req.Env, req.Isolate = spec.Env, &spec
	}
	rctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	res, err := o.Grader.Spawn(rctx, req)
	switch {
	case errors.Is(rctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		return "", "timeout", nil
	case res.ExitCode != 0:
		return "", fmt.Sprintf("exit %d", res.ExitCode), nil
	case err != nil:
		return "", "", fmt.Errorf("grader: %w", err)
	}
	return res.Stdout, "", nil
}
