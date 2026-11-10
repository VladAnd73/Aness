package evalharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSetReadsEveryScenarioDir(t *testing.T) {
	got, err := LoadSet("testdata/sets/basic")
	if err != nil {
		t.Fatalf("LoadSet: %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("scenarios = %+v, want alpha then beta", got)
	}
	if got[1].Prompt != "Write done.txt." || len(got[1].Checks) != 1 {
		t.Errorf("beta = %+v", got[1])
	}
	if !strings.HasSuffix(got[1].Dir, filepath.Join("basic", "beta")) {
		t.Errorf("beta Dir = %q", got[1].Dir)
	}
	if len(got[0].Services) != 1 || got[0].Services[0].Name != "notion-create-pages" {
		t.Errorf("alpha services = %+v", got[0].Services)
	}
	if !got[0].Checks[1].Negate {
		t.Errorf("alpha check 1 lost negate: %+v", got[0].Checks[1])
	}
}

func TestLoadSetRejects(t *testing.T) {
	cases := []struct {
		name, json, rubric, want string
	}{
		{"unknown-kind", `{"prompt":"p","checks":[{"kind":"vibes"}]}`, "", "unknown check kind"},
		{"llm-judge-unflagged", `{"prompt":"p","checks":[{"kind":"llm-judge","rubric":"rubric.md"}]}`, "rubric", "llm_judge"},
		{"llm-judge-no-rubric-file", `{"prompt":"p","llm_judge":true,"checks":[{"kind":"llm-judge","rubric":"rubric.md"}]}`, "", "rubric"},
		{"absolute-path", `{"prompt":"p","checks":[{"kind":"file-exists","path":"/etc/passwd"}]}`, "", "path"},
		{"dotdot-path", `{"prompt":"p","checks":[{"kind":"file-matches","path":"../x","pattern":"a"}]}`, "", "path"},
		{"bad-regexp", `{"prompt":"p","checks":[{"kind":"file-matches","path":"x","pattern":"("}]}`, "", "pattern"},
		{"name-mismatch", `{"name":"other","prompt":"p","checks":[{"kind":"file-exists","path":"x"}]}`, "", "name"},
		{"no-prompt", `{"checks":[{"kind":"file-exists","path":"x"}]}`, "", "prompt"},
		{"no-checks", `{"prompt":"p"}`, "", "checks"},
		{"unknown-field", `{"prompt":"p","chekcs":[]}`, "", "chekcs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set := t.TempDir()
			dir := filepath.Join(set, c.name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "scenario.json"), []byte(c.json), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.rubric != "" {
				if err := os.WriteFile(filepath.Join(dir, "rubric.md"), []byte(c.rubric), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := LoadSet(set)
			if err == nil {
				t.Fatal("LoadSet accepted it")
			}
			if !strings.Contains(err.Error(), c.name) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not name the scenario %q and %q", err, c.name, c.want)
			}
		})
	}
}

func TestLoadSetRejectsAnEmptySet(t *testing.T) {
	if _, err := LoadSet(t.TempDir()); err == nil {
		t.Fatal("LoadSet accepted a set with no scenarios")
	}
}

func TestLoadSetAcceptsTheAcceptanceSuiteSets(t *testing.T) {
	for _, dir := range []string{
		"testdata/notion-set/main",
		"../research/testdata/drafts/main",
		"../research/testdata/drafts/holdout",
		"../../cmd/spore/testdata/research-e2e/drafts/main",
		"../../cmd/spore/testdata/research-e2e/drafts/holdout",
	} {
		if _, err := LoadSet(dir); err != nil {
			t.Errorf("LoadSet(%s): %v", dir, err)
		}
	}
}
