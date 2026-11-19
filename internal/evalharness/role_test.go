package evalharness

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/versality/spore/internal/isolate"
)

const roleBody = "---\nname: qa-analyst\n---\n# qa-analyst\nWrite a test plan.\n"

func roleStep() *RoleStep {
	return &RoleStep{
		Loop: "qa", Step: "intake", Role: "qa-analyst",
		Output: "intake/round-{round}.json", Shape: "json",
		Required: []string{"summary", "plan_path"},
		Skills:   []string{"plan-skill"},
	}
}

// writeRoleScenario writes one role scenario: scenario.json plus files,
// each keyed by its path relative to the scenario directory.
func writeRoleScenario(t *testing.T, set, name, scenarioJSON string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(set, name)
	files["scenario.json"] = scenarioJSON
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func roleFiles(extra map[string]string) map[string]string {
	files := map[string]string{
		"spec.md":                    "Feature FAKE-1: rename a listing.\n",
		"snapshot/src/listing.ts":    "export const title = 'x'\n",
		"grader/repro.md":            "REPRO-SECRET-42: rename with an empty title; the save button stays enabled.\n",
		"snapshot/playwright/a.ts":   "export const a = 1\n",
		"snapshot/playwright/b.json": "{}\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func roleOpts(t *testing.T, set string, sp, grader AgentSpawner) JudgeOptions {
	t.Helper()
	t.Setenv(AllowUnsandboxedEnv, "1")
	cand := filepath.Join(t.TempDir(), "qa-analyst.md")
	if err := os.WriteFile(cand, []byte(roleBody), 0o644); err != nil {
		t.Fatal(err)
	}
	frozen := t.TempDir()
	skill := filepath.Join(frozen, "skills", "plan-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: plan-skill\n---\nplan things\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return JudgeOptions{
		SetDir: set, Candidate: cand, FrozenDir: frozen, Role: roleStep(), Isolation: "none",
		SandboxParent: t.TempDir(), Spawner: sp, Grader: grader, Repeats: 1, Parallel: 1,
	}
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// writeAnalystOutput is what a well-behaved qa-analyst leaves behind:
// the step output and the plan it names.
func writeAnalystOutput(t *testing.T, req SpawnRequest, plan string) {
	t.Helper()
	task := envValue(req.Env, "SPORE_TASK_DIR")
	if task == "" {
		t.Errorf("no SPORE_TASK_DIR in the agent env")
		return
	}
	os.MkdirAll(filepath.Join(task, "intake"), 0o755)
	os.WriteFile(filepath.Join(task, "intake", "plan.md"), []byte(plan), 0o644)
	os.WriteFile(filepath.Join(task, "intake", "round-1.json"), []byte(`{"summary":"s","plan_path":"intake/plan.md"}`), 0o644)
}

func graderReply(reply string) *scriptSpawner {
	return &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		return SpawnResult{Stdout: reply}, nil
	}}
}

// Acceptance 3: the agent's sandbox holds the snapshot, the spec and
// the frozen skills, and neither grader/ nor any .git directory; the
// grader, outside it, still reads grader/.
func TestAcceptanceRole3SandboxHidesGraderAndGit(t *testing.T) {
	requireBwrap(t)
	longLivedCreds(t)
	set := t.TempDir()
	scDir := writeRoleScenario(t, set, "bug1",
		`{"kind":"role","llm_judge":true,"checks":[{"kind":"bug-caught","repro":"grader/repro.md"}]}`, roleFiles(nil))
	var view, taskDir string
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		if r.Isolate == nil {
			t.Error("role run handed the spawner no sandbox spec")
			return SpawnResult{Stdout: resultLine("done", 0)}, nil
		}
		walk := `find / \( -path /nix -o -path /proc -o -path /sys -o -path /dev -o -path /etc -o -path /run \) -prune -o -print 2>/dev/null; printf 'TASK=%s\n' "$SPORE_TASK_DIR"`
		cmd, err := isolate.Command(ctx, *r.Isolate, []string{"/bin/sh", "-c", walk})
		if err != nil {
			t.Errorf("isolate.Command: %v", err)
			return SpawnResult{Stdout: resultLine("done", 0)}, nil
		}
		out, _ := cmd.CombinedOutput()
		for _, f := range cmd.ExtraFiles {
			f.Close()
		}
		view = string(out)
		for _, line := range strings.Split(view, "\n") {
			if v, ok := strings.CutPrefix(line, "TASK="); ok {
				taskDir = v
			}
		}
		writeAnalystOutput(t, r, "- scenario: rename\n")
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	grader := graderReply("QUOTE: - scenario: rename\nVERDICT: CAUGHT\n")
	o := roleOpts(t, set, sp, grader)
	t.Setenv(AllowUnsandboxedEnv, "")
	o.Isolation = "bwrap"
	if _, err := RunJudge(context.Background(), o); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	paths := strings.Split(view, "\n")
	has := func(suffix string) bool {
		return slices.ContainsFunc(paths, func(p string) bool { return strings.HasSuffix(p, suffix) })
	}
	for _, want := range []string{"/src/listing.ts", "/playwright/a.ts", "/.claude/skills/plan-skill/SKILL.md"} {
		if !has(want) {
			t.Errorf("sandbox view lacks %s:\n%s", want, view)
		}
	}
	if taskDir == "" || !slices.Contains(paths, filepath.Join(taskDir, "spec.md")) {
		t.Errorf("sandbox view lacks $SPORE_TASK_DIR/spec.md (task dir %q)", taskDir)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "/.git") || strings.Contains(p, "/.git/") || strings.Contains(p, "repro.md") || strings.HasPrefix(p, scDir) {
			t.Errorf("sandbox view shows %s", p)
		}
	}
	if len(grader.reqs) != 1 || !strings.Contains(grader.reqs[0].Prompt, "REPRO-SECRET-42") {
		t.Errorf("grader was not given the repro steps from grader/ (requests %d)", len(grader.reqs))
	}

	bad := t.TempDir()
	writeRoleScenario(t, bad, "leak", `{"kind":"role","checks":[{"kind":"output-valid"}]}`,
		roleFiles(map[string]string{"snapshot/.git/HEAD": "ref: refs/heads/main\n"}))
	if _, err := LoadSet(bad); err == nil || !strings.Contains(err.Error(), ".git") {
		t.Errorf("LoadSet on a snapshot holding .git = %v, want a refusal naming .git", err)
	}
}

// Acceptance 4: the prompt is the step note then the candidate body,
// and the env names the task dir, role and step skills.
func TestAcceptanceRole4PromptAndEnv(t *testing.T) {
	set := t.TempDir()
	writeRoleScenario(t, set, "bug1", `{"kind":"role","checks":[{"kind":"output-valid"}]}`, roleFiles(nil))
	var req SpawnRequest
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		req = r
		writeAnalystOutput(t, r, "- scenario: rename\n")
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	rep, err := RunJudge(context.Background(), roleOpts(t, set, sp, nil))
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	task := envValue(req.Env, "SPORE_TASK_DIR")
	output := filepath.Join(task, "intake", "round-{round}.json")
	note := StepNote(*roleStep(), output, 1)
	if note == "" || req.Prompt != note+"\n\n"+roleBody {
		t.Errorf("prompt = %q\nwant the step note %q, a blank line, then the candidate body", req.Prompt, note)
	}
	if !strings.Contains(req.Prompt, output) || !strings.Contains(req.Prompt, `"summary", "plan_path"`) {
		t.Errorf("prompt does not name the output path %s and its required keys", output)
	}
	if b, err := os.ReadFile(filepath.Join(task, "spec.md")); err != nil || !strings.Contains(string(b), "FAKE-1") {
		t.Errorf("$SPORE_TASK_DIR/spec.md = %q, %v", b, err)
	}
	if got := envValue(req.Env, "SPORE_ROLE"); got != "qa-analyst" {
		t.Errorf("SPORE_ROLE = %q", got)
	}
	if got := envValue(req.Env, "SPORE_STEP_SKILLS"); got != "plan-skill" {
		t.Errorf("SPORE_STEP_SKILLS = %q", got)
	}
	if rep.Passed != 1 {
		t.Errorf("output-valid run did not pass: %+v", rep.Scenarios)
	}
}

func runRole(t *testing.T, scenarioJSON string, extra map[string]string, agent func(t *testing.T, r SpawnRequest), grader AgentSpawner) RunReport {
	t.Helper()
	set := t.TempDir()
	writeRoleScenario(t, set, "bug1", scenarioJSON, roleFiles(extra))
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		agent(t, r)
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	rep, err := RunJudge(context.Background(), roleOpts(t, set, sp, grader))
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	return firstRun(t, rep)
}

// Acceptance 6: a "yes" with no quoted scenario is not caught; a quote
// found in the output is.
func TestAcceptanceRole6CaughtNeedsAQuote(t *testing.T) {
	sc := `{"kind":"role","llm_judge":true,"checks":[{"kind":"bug-caught","repro":"grader/repro.md"}]}`
	plan := func(t *testing.T, r SpawnRequest) {
		writeAnalystOutput(t, r, "- scenario: rename to an empty title is refused\n")
	}
	for _, c := range []struct {
		name, reply string
		want        bool
	}{
		{"yes without quote", "yes\n", false},
		{"caught without quote", "VERDICT: CAUGHT\n", false},
		{"quote not in the output", "QUOTE: - scenario: delete the listing\nVERDICT: CAUGHT\n", false},
		{"quoted", "QUOTE: rename to an empty title is refused\nVERDICT: CAUGHT\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := runRole(t, sc, nil, plan, graderReply(c.reply))
			if run.Pass != c.want {
				t.Errorf("pass = %v (%s), want %v", run.Pass, run.Reason, c.want)
			}
			if !c.want && !strings.Contains(run.Reason, "bug-caught") {
				t.Errorf("reason %q does not name bug-caught", run.Reason)
			}
		})
	}
}

// Acceptance 7: more scenarios than 1.5x the reasonable count fails
// fluff even when the bug is caught.
func TestAcceptanceRole7FluffCapsScenarioCount(t *testing.T) {
	sc := `{"kind":"role","llm_judge":true,"checks":[
		{"kind":"output-valid"},
		{"kind":"bug-caught","repro":"grader/repro.md"},
		{"kind":"fluff","path":"intake/plan.md","pattern":"^- scenario:","reasonable":2}]}`
	grader := graderReply("QUOTE: - scenario: empty title\nVERDICT: CAUGHT\n")
	for _, c := range []struct {
		n    int
		want bool
	}{{3, true}, {4, false}} {
		plan := "- scenario: empty title\n" + strings.Repeat("- scenario: other\n", c.n-1)
		run := runRole(t, sc, nil, func(t *testing.T, r SpawnRequest) { writeAnalystOutput(t, r, plan) }, grader)
		if run.Pass != c.want {
			t.Errorf("%d scenarios: pass = %v (%s), want %v", c.n, run.Pass, run.Reason, c.want)
		}
		if !c.want && !strings.Contains(run.Reason, "fluff") {
			t.Errorf("%d scenarios: reason %q does not name fluff", c.n, run.Reason)
		}
	}
}

// Acceptance 8: each mechanical code-quality rule fails a spec that
// breaks it, naming the rule.
func TestAcceptanceRole8CodeQualityRules(t *testing.T) {
	sc := `{"kind":"role","checks":[{"kind":"code-quality"}]}`
	snapshot := map[string]string{
		"snapshot/playwright/pages/listing.ts": "export class ListingPage {\n  async openListing(id: string) {\n    await this.page.goto(`/listings/${id}`)\n  }\n}\n",
	}
	const head = "import { test, expect } from '@playwright/test'\n\n"
	for _, c := range []struct{ rule, spec string }{
		{"", head + "test('renames a listing', async ({ page }) => {\n  await page.getByLabel('Title').fill('Renamed')\n  await expect(page.getByRole('heading')).toHaveText('Renamed')\n})\n"},
		{"no-added-comments", head + "test('renames a listing', async ({ page }) => {\n  // fill in the new title\n  await page.getByLabel('Title').fill('Renamed')\n  await expect(page.getByRole('heading')).toHaveText('Renamed')\n})\n"},
		{"no-visibility-only-asserts", head + "test('renames a listing', async ({ page }) => {\n  await page.getByLabel('Title').fill('Renamed')\n  await expect(page.getByText('Saved')).toBeVisible()\n})\n"},
		{"no-duplicate-helpers", head + "async function openListing(page, id) {\n  await page.goto(`/listings/${id}`)\n}\n\ntest('renames a listing', async ({ page }) => {\n  await openListing(page, '1')\n  await expect(page.getByRole('heading')).toHaveText('Renamed')\n})\n"},
	} {
		name := c.rule
		if name == "" {
			name = "clean"
		}
		t.Run(name, func(t *testing.T) {
			run := runRole(t, sc, snapshot, func(t *testing.T, r SpawnRequest) {
				p := filepath.Join(r.WorkDir, "playwright", "tests", "listing.spec.ts")
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.WriteFile(p, []byte(c.spec), 0o644)
			}, nil)
			if c.rule == "" {
				if !run.Pass {
					t.Errorf("clean spec failed: %s", run.Reason)
				}
				return
			}
			if run.Pass || !strings.Contains(run.Reason, c.rule) {
				t.Errorf("pass = %v, reason %q; want a failure naming %s", run.Pass, run.Reason, c.rule)
			}
		})
	}
}
