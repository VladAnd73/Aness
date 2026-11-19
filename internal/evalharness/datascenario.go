package evalharness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/versality/spore/internal/research/fakemcp"
)

// DataScenario is one scenario loaded from <set>/<name>/scenario.json.
// Files under <set>/<name>/files/ are copied into the run's working
// directory before the skill is injected, so a fixture never overwrites
// the candidate. Name is the directory name; a name in the JSON must
// agree with it. An llm-judge check needs LLMJudge set, so a set never
// grades by a model without saying so up front.
type DataScenario struct {
	Name     string          `json:"name"`
	Dir      string          `json:"-"`
	Prompt   string          `json:"prompt"`
	Services []fakemcp.Tool  `json:"services,omitempty"`
	Checks   []ScenarioCheck `json:"checks"`
	LLMJudge bool            `json:"llm_judge,omitempty"`
}

// ScenarioCheck is one hard check on a finished run. Kind is one of
// file-exists, file-matches, service-call, tool-used, reply-contains,
// or llm-judge; Negate flips file-exists, service-call, tool-used and
// reply-contains into their "not" form.
type ScenarioCheck struct {
	Kind    string            `json:"kind"`
	Path    string            `json:"path,omitempty"`
	Pattern string            `json:"pattern,omitempty"`
	Tool    string            `json:"tool,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
	Text    string            `json:"text,omitempty"`
	Rubric  string            `json:"rubric,omitempty"`
	Negate  bool              `json:"negate,omitempty"`
}

// LoadSet loads every scenario directory under dir, sorted by name.
func LoadSet(dir string) ([]DataScenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("evalharness: scenario set: %w", err)
	}
	var out []DataScenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sc, err := loadScenario(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("evalharness: scenario set %s: no scenarios", dir)
	}
	return out, nil
}

func loadScenario(dir string) (DataScenario, error) {
	name := filepath.Base(dir)
	fail := func(format string, a ...any) (DataScenario, error) {
		return DataScenario{}, fmt.Errorf("evalharness: scenario %s: %s", dir, fmt.Sprintf(format, a...))
	}
	raw, err := os.ReadFile(filepath.Join(dir, "scenario.json"))
	if err != nil {
		return fail("%v", err)
	}
	var sc DataScenario
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sc); err != nil {
		return fail("scenario.json: %v", err)
	}
	if sc.Name != "" && sc.Name != name {
		return fail("name %q does not match its directory", sc.Name)
	}
	sc.Name, sc.Dir = name, dir
	if strings.TrimSpace(sc.Prompt) == "" {
		return fail("empty prompt")
	}
	if len(sc.Checks) == 0 {
		return fail("no checks")
	}
	for i, s := range sc.Services {
		if s.Server == "" || s.Name == "" {
			return fail("service %d: server and name are required", i)
		}
	}
	for i, c := range sc.Checks {
		if err := validateCheck(sc, c); err != nil {
			return fail("check %d (%s): %v", i, c.Kind, err)
		}
	}
	return sc, nil
}

func validateCheck(sc DataScenario, c ScenarioCheck) error {
	switch c.Kind {
	case "file-exists", "file-matches":
		if c.Path == "" || filepath.IsAbs(c.Path) || !filepath.IsLocal(c.Path) {
			return fmt.Errorf("path %q must be relative and stay inside the run dir", c.Path)
		}
		if c.Kind == "file-matches" {
			if c.Negate {
				return fmt.Errorf("negate is not supported")
			}
			if c.Pattern == "" {
				return fmt.Errorf("empty pattern")
			}
			if _, err := regexp.Compile(c.Pattern); err != nil {
				return fmt.Errorf("pattern: %v", err)
			}
		}
	case "service-call", "tool-used":
		if c.Tool == "" {
			return fmt.Errorf("empty tool")
		}
	case "reply-contains":
		if c.Text == "" {
			return fmt.Errorf("empty text")
		}
	case "llm-judge":
		if !sc.LLMJudge {
			return fmt.Errorf("scenario must set llm_judge: true")
		}
		if c.Rubric == "" || !filepath.IsLocal(c.Rubric) {
			return fmt.Errorf("rubric %q must name a file in the scenario dir", c.Rubric)
		}
		if _, err := os.Stat(filepath.Join(sc.Dir, c.Rubric)); err != nil {
			return fmt.Errorf("rubric: %v", err)
		}
	default:
		return fmt.Errorf("unknown check kind %q", c.Kind)
	}
	return nil
}
