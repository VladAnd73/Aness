package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/versality/spore/internal/research"
)

// copyFixtureProject copies testdata/research-roles-e2e/project into a
// fresh git repo and returns its root.
func copyFixtureProject(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("testdata/research-roles-e2e/project")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "proj")
	if out, err := exec.Command("cp", "-r", src, root).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v: %s", err, out)
	}
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=trunk"}, {"config", "user.name", "operator"},
		{"config", "user.email", "operator@localhost"}, {"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@localhost", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return root
}

func roleResearchEnv(t *testing.T) (store research.Store, goal string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("SPORE_RESEARCH_DIR", "")
	goal = filepath.Join(root, "goal.md")
	if err := os.WriteFile(goal, []byte("Find more bugs.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return research.Store{Root: filepath.Join(root, "state", "spore", "research")}, goal
}

func runResearchCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = researchMain(&out, &errOut, args, researchDeps{})
	return code, out.String(), errOut.String()
}

// Acceptance 1: a role target records the step and freezes its skills;
// a role target that is not markdown is refused.
func TestAcceptanceRole1NewRoleCampaign(t *testing.T) {
	store, goal := roleResearchEnv(t)
	proj := copyFixtureProject(t)
	target := filepath.Join(proj, "bootstrap", "roles", "qa-analyst.md")
	code, out, errOut := runResearchCLI("new", "c", "--target", target, "--kind", "role", "--step", "intake", "--goal", goal)
	if code != 0 {
		t.Fatalf("new: exit %d\n%s%s", code, out, errOut)
	}
	cfg, err := store.Load("c")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kind != "role" || cfg.Step != "intake" || cfg.StepOutput != "intake/round-{round}.json" || cfg.StepShape != "json" {
		t.Errorf("kind %q step %q output %q shape %q", cfg.Kind, cfg.Step, cfg.StepOutput, cfg.StepShape)
	}
	if strings.Join(cfg.StepSkills, ",") != "plan-skill,builtin-skill" {
		t.Errorf("step skills = %v", cfg.StepSkills)
	}
	frozen := filepath.Join(store.Dir("c"), "repo", "context", "skills", "plan-skill", "SKILL.md")
	if b, err := os.ReadFile(frozen); err != nil || !strings.Contains(string(b), "plan-skill") {
		t.Errorf("frozen skill %s = %q, %v", frozen, b, err)
	}
	if !strings.Contains(out+errOut, "builtin-skill") {
		t.Errorf("no warning names the skill that was not found:\n%s%s", out, errOut)
	}

	notes := filepath.Join(proj, "notes.txt")
	os.WriteFile(notes, []byte("x\n"), 0o600)
	code, _, errOut = runResearchCLI("new", "d", "--target", notes, "--kind", "role", "--step", "intake", "--goal", goal)
	if code == 0 || !strings.Contains(errOut, ".md") {
		t.Errorf("a non-markdown role target: exit %d, stderr %q; want a refusal naming .md", code, errOut)
	}
}

// Acceptance 2: without --kind (or with --kind skill) a target not named
// SKILL.md is refused exactly as before.
func TestAcceptanceRole2SkillKindStillNeedsSkillMD(t *testing.T) {
	_, goal := roleResearchEnv(t)
	target := filepath.Join(t.TempDir(), "qa-analyst.md")
	os.WriteFile(target, []byte("x\n"), 0o600)
	const want = "spore research new: v1 improves skills only: --target must be a file named SKILL.md, got qa-analyst.md\n"
	for _, extra := range [][]string{nil, {"--kind", "skill"}} {
		args := append([]string{"new", "c", "--target", target, "--goal", goal}, extra...)
		code, _, errOut := runResearchCLI(args...)
		if code != 1 || errOut != want {
			t.Errorf("new %v: exit %d, stderr %q; want exit 1 and %q", extra, code, errOut, want)
		}
	}
}

// Acceptance 13: judge draft on a role campaign mints the corpus
// builder brief.
func TestAcceptanceRole13JudgeDraftRoleBrief(t *testing.T) {
	_, goal := roleResearchEnv(t)
	proj := copyFixtureProject(t)
	target := filepath.Join(proj, "bootstrap", "roles", "qa-analyst.md")
	if code, out, errOut := runResearchCLI("new", "c", "--target", target, "--kind", "role", "--step", "intake", "--goal", goal); code != 0 {
		t.Fatalf("new: exit %d\n%s%s", code, out, errOut)
	}
	t.Chdir(t.TempDir())
	code, out, errOut := runResearchCLI("judge", "draft", "c")
	if code != 0 {
		t.Fatalf("judge draft: exit %d\n%s", code, errOut)
	}
	b, err := os.ReadFile(filepath.Join("tasks", strings.TrimSpace(out)+".md"))
	if err != nil {
		t.Fatal(err)
	}
	brief := string(b)
	for _, leak := range []string{"HK", "linear-bug-report", "qa-author", "assistant coordinator"} {
		if strings.Contains(brief, leak) {
			t.Errorf("brief names the consumer-specific %q", leak)
		}
	}
	frozen := regexp.MustCompile(`--frozen (\S+)`).FindStringSubmatch(brief)
	if frozen == nil || !dirExists(frozen[1]) {
		t.Errorf("brief's --frozen path %v does not exist", frozen)
	}
	for _, want := range []string{
		"status: draft",
		"filed by your own QA", "reported by customers and developers",
		"linked feature ticket", "repro steps", "identifiable fix",
		"pre-fix commit",
		"at least 30%", "at least 5 bugs", "same side",
		"Discrimination proof", "weakened",
		"Never commit corpus content",
		"snapshot/", "grader/", "spec.md", `"kind": "role"`,
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief lacks %q", want)
		}
	}
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
