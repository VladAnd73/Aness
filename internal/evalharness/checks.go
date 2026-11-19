package evalharness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/versality/spore/internal/research/fakemcp"
)

// RunArtifacts is what one finished run left behind for grading.
type RunArtifacts struct {
	RunWD  string
	Stream StreamResult
	Calls  []fakemcp.Call
}

// GradeChecks runs every code check against a. failed names each
// failing check by index and kind only: the result is editor feedback
// material and must never carry a check's text. An llm-judge check is
// an error here, so no caller can skip one by accident.
func GradeChecks(a RunArtifacts, checks []ScenarioCheck) (pass bool, failed []string, err error) {
	for i, c := range checks {
		ok, err := gradeCheck(a, c)
		if err != nil {
			return false, nil, fmt.Errorf("evalharness: check %d (%s): %w", i, c.Kind, err)
		}
		if !ok {
			failed = append(failed, fmt.Sprintf("check %d (%s)", i, c.Kind))
		}
	}
	return len(failed) == 0, failed, nil
}

func gradeCheck(a RunArtifacts, c ScenarioCheck) (bool, error) {
	var hit bool
	switch c.Kind {
	case "file-exists":
		_, err := os.Stat(filepath.Join(a.RunWD, c.Path))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		hit = err == nil
	case "file-matches":
		b, err := os.ReadFile(filepath.Join(a.RunWD, c.Path))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		re, err := regexp.Compile("(?m)" + c.Pattern)
		if err != nil {
			return false, err
		}
		hit = re.Match(b)
	case "service-call":
		hit = slices.ContainsFunc(a.Calls, func(call fakemcp.Call) bool {
			return call.Tool == c.Tool && argsMatch(call.Args, c.Fields)
		})
	case "tool-used":
		hit = slices.Contains(a.Stream.ToolsUsed, c.Tool)
	case "reply-contains":
		hit = strings.Contains(a.Stream.FinalReply, c.Text)
	case "llm-judge":
		return false, errors.New("llm-judge is graded by the judge's grader, not by GradeChecks")
	default:
		return false, fmt.Errorf("unknown check kind %q", c.Kind)
	}
	return hit != c.Negate, nil
}

// argsMatch is a subset match: every wanted field is present and its
// value, rendered as a string, equals the wanted string. A non-string
// value compares by its JSON text, so 2 matches "2".
func argsMatch(args map[string]any, want map[string]string) bool {
	for k, w := range want {
		v, ok := args[k]
		if !ok {
			return false
		}
		s, isStr := v.(string)
		if !isStr {
			b, err := json.Marshal(v)
			if err != nil {
				return false
			}
			s = string(b)
		}
		if s != w {
			return false
		}
	}
	return true
}
