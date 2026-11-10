package evalharness

import "github.com/versality/spore/internal/research/fakemcp"

// DataScenario is one scenario loaded from <set>/<name>/scenario.json.
// Files under <set>/<name>/files/ are copied into the run's working
// directory before the agent starts.
type DataScenario struct {
	Name     string          `json:"name"`
	Prompt   string          `json:"prompt"`
	Services []fakemcp.Tool  `json:"services,omitempty"`
	Checks   []ScenarioCheck `json:"checks"`
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

func LoadSet(dir string) ([]DataScenario, error) { return nil, nil }
