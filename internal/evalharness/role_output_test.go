package evalharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output-valid agrees with the loop's own parser: a verdict whose author
// is not the step's fails, as the loop would refuse it.
func TestOutputValidUsesTheLoopParser(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "round-1.json")
	rs := &RoleStep{Step: "review", Role: "qa-reviewer", Output: "review/round-{round}.json", Shape: "verdict", Author: "R"}
	for _, c := range []struct {
		body string
		want bool
	}{
		{`{"author":"R","verdict":"approve","summary":"ok","comments":[]}`, true},
		{`{"author":"someone-else","verdict":"approve","summary":"ok","comments":[]}`, false},
		{`{"author":"R","verdict":"approve","summary":"ok","comments":[],"extra":1}`, false},
	} {
		os.WriteFile(out, []byte(c.body), 0o644)
		ok, err := outputValid(RunArtifacts{Role: rs, Output: out})
		if err != nil || ok != c.want {
			t.Errorf("%s: ok %v err %v, want %v", c.body, ok, err, c.want)
		}
	}
}

// The grader's material skips node_modules and puts new files first, so
// a large changed fixture cannot push the new test out of the cap.
func TestRoleMaterialKeepsNewTestsUnderTheCap(t *testing.T) {
	snap, run := t.TempDir(), t.TempDir()
	write := func(root, rel, body string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write(snap, "a-big.json", "old\n")
	write(run, "a-big.json", strings.Repeat("x", maxMaterial+10))
	write(run, "node_modules/pkg/index.js", "NODE-MODULES-BODY\n")
	write(run, "tests/rename.spec.ts", "NEW-SPEC-BODY\n")
	m, err := roleMaterial(snap, run, filepath.Join(run, ".spore", "replay"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m, "NEW-SPEC-BODY") || strings.Contains(m, "NODE-MODULES-BODY") {
		t.Errorf("material keeps the new spec %v, skips node_modules %v", strings.Contains(m, "NEW-SPEC-BODY"), !strings.Contains(m, "NODE-MODULES-BODY"))
	}
}

// A json step with transitions wants a verdict the loop can route on;
// any other value would stall the loop.
func TestOutputValidWantsARoutableVerdict(t *testing.T) {
	out := filepath.Join(t.TempDir(), "round-1.json")
	rs := &RoleStep{Step: "check", Role: "checker", Output: "check/round-{round}.json", Shape: "json",
		Required: []string{"summary"}, Verdicts: []string{"pass", "fail"}}
	for body, want := range map[string]bool{
		`{"summary":"s","verdict":"pass"}`:  true,
		`{"summary":"s","verdict":"maybe"}`: false,
		`{"summary":"s"}`:                   false,
	} {
		os.WriteFile(out, []byte(body), 0o644)
		if ok, err := outputValid(RunArtifacts{Role: rs, Output: out}); err != nil || ok != want {
			t.Errorf("%s: ok %v err %v, want %v", body, ok, err, want)
		}
	}
}

// A large file the agent never touched is not the agent's output.
func TestRoleMaterialSkipsUnchangedLargeFiles(t *testing.T) {
	snap, run := t.TempDir(), t.TempDir()
	big := strings.Repeat("y", maxMaterial+10)
	for _, root := range []string{snap, run} {
		os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(big), 0o644)
	}
	m, err := roleMaterial(snap, run, filepath.Join(run, ".spore", "replay"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m, "package-lock.json") {
		t.Errorf("material lists the unchanged lock file:\n%.200s", m)
	}
}
