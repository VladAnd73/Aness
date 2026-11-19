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

// RunArtifacts is what one finished run left behind for grading. A
// role run also names its pinned step, its task dir, the step output
// it was told to write and the snapshot it started from.
type RunArtifacts struct {
	RunWD  string
	Stream StreamResult
	Calls  []fakemcp.Call

	Role                      *RoleStep
	TaskDir, Output, Snapshot string
}

// GradeChecks runs every code check against a. failed names each
// failing check by index and kind only: the result is editor feedback
// material and must never carry a check's text. An llm-judge check is
// an error here, so no caller can skip one by accident.
func GradeChecks(a RunArtifacts, checks []ScenarioCheck) (pass bool, failed []string, err error) {
	for i, c := range checks {
		ok, detail, err := gradeCheck(a, c)
		if err != nil {
			return false, nil, fmt.Errorf("evalharness: check %d (%s): %w", i, c.Kind, err)
		}
		if !ok {
			label := c.Kind
			if detail != "" {
				label += ": " + detail
			}
			failed = append(failed, fmt.Sprintf("check %d (%s)", i, label))
		}
	}
	return len(failed) == 0, failed, nil
}

// gradeCheck's detail names the rules a code-quality check found
// broken; rule names are no scenario text.
func gradeCheck(a RunArtifacts, c ScenarioCheck) (ok bool, detail string, err error) {
	var hit bool
	switch c.Kind {
	case "file-exists":
		_, err := os.Stat(filepath.Join(a.RunWD, c.Path))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, "", err
		}
		hit = err == nil
	case "file-matches":
		b, err := os.ReadFile(filepath.Join(a.RunWD, c.Path))
		if errors.Is(err, os.ErrNotExist) {
			return false, "", nil
		}
		if err != nil {
			return false, "", err
		}
		re, err := regexp.Compile("(?m)" + c.Pattern)
		if err != nil {
			return false, "", err
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
	case "output-valid", "fluff", "code-quality":
		if a.Role == nil {
			return false, "", fmt.Errorf("%s grades a role run only", c.Kind)
		}
		switch c.Kind {
		case "output-valid":
			ok, err = outputValid(a)
		case "fluff":
			ok, err = fluffCap(a, c)
		default:
			var broken []string
			broken, err = qualityFindings(a.Snapshot, a.RunWD, c.Rules)
			ok, detail = len(broken) == 0, strings.Join(broken, ", ")
		}
		return ok, detail, err
	case "llm-judge", "bug-caught":
		return false, "", errors.New(c.Kind + " is graded by the judge's grader, not by GradeChecks")
	default:
		return false, "", fmt.Errorf("unknown check kind %q", c.Kind)
	}
	return hit != c.Negate, "", nil
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
