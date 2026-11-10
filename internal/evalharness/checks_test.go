package evalharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/research/fakemcp"
)

func TestGradeChecks(t *testing.T) {
	wd := t.TempDir()
	if err := os.WriteFile(filepath.Join(wd, "out.txt"), []byte("head\nstatus: done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := RunArtifacts{
		RunWD:  wd,
		Stream: StreamResult{FinalReply: "All PROBE-DONE here", ToolsUsed: []string{"Skill", "Write"}},
		Calls: []fakemcp.Call{{Server: "claude_ai_Notion", Tool: "notion-create-pages",
			Args: map[string]any{"title": "Nightly", "count": float64(2)}}},
	}
	cases := []struct {
		name  string
		check ScenarioCheck
		pass  bool
	}{
		{"file present", ScenarioCheck{Kind: "file-exists", Path: "out.txt"}, true},
		{"file absent", ScenarioCheck{Kind: "file-exists", Path: "nope.txt"}, false},
		{"file absent negated", ScenarioCheck{Kind: "file-exists", Path: "nope.txt", Negate: true}, true},
		{"file present negated", ScenarioCheck{Kind: "file-exists", Path: "out.txt", Negate: true}, false},
		{"regexp hit multiline", ScenarioCheck{Kind: "file-matches", Path: "out.txt", Pattern: "^status: done$"}, true},
		{"regexp miss", ScenarioCheck{Kind: "file-matches", Path: "out.txt", Pattern: "^status: todo$"}, false},
		{"regexp on missing file", ScenarioCheck{Kind: "file-matches", Path: "nope.txt", Pattern: "x"}, false},
		{"call matching fields", ScenarioCheck{Kind: "service-call", Tool: "notion-create-pages", Fields: map[string]string{"title": "Nightly", "count": "2"}}, true},
		{"call non-matching field", ScenarioCheck{Kind: "service-call", Tool: "notion-create-pages", Fields: map[string]string{"title": "Daily"}}, false},
		{"call missing field", ScenarioCheck{Kind: "service-call", Tool: "notion-create-pages", Fields: map[string]string{"parent": "x"}}, false},
		{"not called", ScenarioCheck{Kind: "service-call", Tool: "notion-search"}, false},
		{"not called negated", ScenarioCheck{Kind: "service-call", Tool: "notion-search", Negate: true}, true},
		{"called negated", ScenarioCheck{Kind: "service-call", Tool: "notion-create-pages", Negate: true}, false},
		{"tool used", ScenarioCheck{Kind: "tool-used", Tool: "Write"}, true},
		{"tool not used", ScenarioCheck{Kind: "tool-used", Tool: "Bash"}, false},
		{"tool not used negated", ScenarioCheck{Kind: "tool-used", Tool: "Bash", Negate: true}, true},
		{"tool used negated", ScenarioCheck{Kind: "tool-used", Tool: "Write", Negate: true}, false},
		{"reply contains", ScenarioCheck{Kind: "reply-contains", Text: "PROBE-DONE"}, true},
		{"reply omits", ScenarioCheck{Kind: "reply-contains", Text: "FAILED"}, false},
		{"reply omits negated", ScenarioCheck{Kind: "reply-contains", Text: "FAILED", Negate: true}, true},
		{"reply contains negated", ScenarioCheck{Kind: "reply-contains", Text: "PROBE-DONE", Negate: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, failed, err := GradeChecks(a, []ScenarioCheck{{Kind: "file-exists", Path: "out.txt"}, c.check})
			if err != nil {
				t.Fatalf("GradeChecks: %v", err)
			}
			if pass != c.pass {
				t.Fatalf("pass = %v, want %v (failed %v)", pass, c.pass, failed)
			}
			if !c.pass && (len(failed) != 1 || failed[0] != "check 1 ("+c.check.Kind+")") {
				t.Errorf("failed = %v, want only check 1 named by index and kind", failed)
			}
		})
	}
}

func TestGradeChecksRefusesLLMJudge(t *testing.T) {
	_, _, err := GradeChecks(RunArtifacts{RunWD: t.TempDir()}, []ScenarioCheck{{Kind: "llm-judge", Rubric: "rubric.md"}})
	if err == nil || !strings.Contains(err.Error(), "llm-judge") {
		t.Fatalf("err = %v, want a refusal naming llm-judge", err)
	}
}
