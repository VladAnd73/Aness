package evalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/versality/spore/internal/task"
)

// outputValid parses the step output with the loop's own parser, so
// the judge accepts exactly what the loop would.
func outputValid(a RunArtifacts) (bool, error) {
	b, err := os.ReadFile(a.Output)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	sf := task.StepFiles{Step: a.Role.Step, Output: a.Role.Output, Shape: a.Role.Shape, Author: a.Role.Author, Role: a.Role.Role, Required: a.Role.Required}
	_, err = task.ParseStepOutput(a.Output, b, sf, false)
	if errors.Is(err, task.ErrInvalidRoleFile) {
		return false, nil
	}
	return err == nil, err
}

// fluffCap fails a plan holding more than 1.5 times the scenario count
// a reasonable plan for the feature has.
func fluffCap(a RunArtifacts, c ScenarioCheck) (bool, error) {
	b, err := os.ReadFile(filepath.Join(a.TaskDir, c.Path))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	re, err := regexp.Compile("(?m)" + c.Pattern)
	if err != nil {
		return false, err
	}
	n := len(re.FindAllIndex(b, -1))
	return n > 0 && 2*n <= 3*c.Reasonable, nil
}

const caughtPrompt = `You decide whether a QA agent's output would have caught a known bug.
The agent never saw the bug. It wrote the output below for the feature.

<repro>
%s
</repro>

<output>
%s
</output>

Does any scenario or test in the output exercise these repro steps
closely enough that running it would have failed on this bug? A
scenario that only names the area, without the triggering input or
step, does not count.

End your answer with exactly two lines. The first quotes, verbatim,
the one line of the output that matches (or "QUOTE: none"):
QUOTE: <the line>
VERDICT: CAUGHT or VERDICT: MISSED
`

// minQuote keeps a one-word quote from matching any output.
const minQuote = 12

// parseCaught reads a bug-caught grader reply. It is caught only when
// the last verdict is CAUGHT and the quote, whitespace and case aside,
// appears in the material the grader was shown.
func parseCaught(reply, material string) (caught bool, quote string) {
	verdict := ""
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		if q, ok := strings.CutPrefix(line, "QUOTE:"); ok {
			quote = strings.TrimSpace(q)
		}
		if strings.HasPrefix(line, "VERDICT:") {
			verdict = line
		}
	}
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	q := norm(quote)
	if verdict != "VERDICT: CAUGHT" || len(q) < minQuote || !strings.Contains(norm(material), q) {
		return false, quote
	}
	return true, quote
}

func gradeCaught(ctx context.Context, o JudgeOptions, sb *Sandbox, reproPath, material string) (caught bool, quote, crash string, err error) {
	repro, err := os.ReadFile(reproPath)
	if err != nil {
		return false, "", "", err
	}
	reply, crash, err := askGrader(ctx, o, sb, fmt.Sprintf(caughtPrompt, strings.TrimSpace(string(repro)), material))
	if err != nil || crash != "" {
		return false, "", crash, err
	}
	caught, quote = parseCaught(reply, material)
	return caught, quote, "", nil
}

func validateRoleCheck(sc DataScenario, c ScenarioCheck) error {
	if sc.Kind != "role" {
		return fmt.Errorf("only a role scenario may use it")
	}
	switch c.Kind {
	case "bug-caught":
		if !sc.LLMJudge {
			return fmt.Errorf("scenario must set llm_judge: true")
		}
		if !strings.HasPrefix(c.Repro, "grader/") || !filepath.IsLocal(c.Repro) {
			return fmt.Errorf("repro %q must name a file under grader/", c.Repro)
		}
		if _, err := os.Stat(filepath.Join(sc.Dir, c.Repro)); err != nil {
			return fmt.Errorf("repro: %v", err)
		}
	case "fluff":
		if !filepath.IsLocal(c.Path) {
			return fmt.Errorf("path %q must be relative to $SPORE_TASK_DIR", c.Path)
		}
		if c.Reasonable <= 0 {
			return fmt.Errorf("reasonable must be the scenario count of a reasonable plan")
		}
		if c.Pattern == "" {
			return fmt.Errorf("empty pattern")
		}
		if _, err := regexp.Compile(c.Pattern); err != nil {
			return fmt.Errorf("pattern: %v", err)
		}
	case "code-quality":
		for _, r := range c.Rules {
			if !slices.Contains(qualityRules, r) {
				return fmt.Errorf("unknown rule %q (want %s)", r, strings.Join(qualityRules, ", "))
			}
		}
	}
	return nil
}

// validateRoleScenario checks what a role run copies in: the spec, and
// a snapshot without git history, which could hold the fix.
func validateRoleScenario(sc DataScenario) error {
	if _, err := os.Stat(filepath.Join(sc.Dir, "spec.md")); err != nil {
		return fmt.Errorf("spec.md: %v", err)
	}
	snap := filepath.Join(sc.Dir, "snapshot")
	if !dirExists(snap) {
		return nil
	}
	return filepath.WalkDir(snap, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			return fmt.Errorf("snapshot holds %s: git history could show the fix", path)
		}
		return nil
	})
}
