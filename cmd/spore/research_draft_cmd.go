package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

const judgeDraftBody = `
**Deliverable:** two scenario sets for research campaign %[1]s, drafted in
the git-ignored folder research-drafts/%[1]s/ of your worktree, plus a
DONE report with the discrimination proof. Do not commit them and do not
start any night.

## The skill

- Live file: %[2]s
- Skill name: %[3]s
- Campaign goal:

%[4]s

## What to write

- research-drafts/%[1]s/main/<scenario>/scenario.json: the scenarios the
  nightly loop scores every edit on. Aim for 3 to 8.
- research-drafts/%[1]s/holdout/<scenario>/scenario.json: at least 5
  different scenarios, sealed until the campaign ends. A winner must beat
  the original on them, so they must leave the original room to fail. Never tune them against the
  skill; they catch an edit that only games the main set.
- The format is the eval judge's data scenario (prompt, checks, optional
  fake services); see docs/autoresearch.md and the example under
  cmd/spore/testdata/research-e2e/drafts.

Ground each scenario in a past failure of this skill. Look for them in
tasks/ (done and blocked briefs naming %[3]s), git log, docs/ and
state.md, and in any worker report that names the skill.

## Discrimination proof (required)

A set that every version passes measures nothing. Before DONE:

1. Run the judge on the current skill:
   spore eval judge --scenarios-root research-drafts/%[1]s --skill %[3]s
   --frozen %[5]s --candidate %[2]s --set main --out /tmp/current.json
2. Make a deliberately broken copy of the skill (remove the instruction
   a scenario depends on) and run the same command on that copy.
3. The broken copy must fail where the current skill passes. Put both
   scores, and what you broke, in your report. Repeat for the holdout.

## Hand-off

The operator approves the sets with:
spore research judge approve %[1]s --from <your worktree>/research-drafts/%[1]s
`

const roleDraftBody = `
**Deliverable:** a bug-replay corpus for research campaign %[1]s, drafted
in the git-ignored folder research-drafts/%[1]s/ of your worktree, plus a
DONE report with the discrimination proof and the dropped-bug list.
Never commit corpus content: ticket text, customer names and repro steps
stay in research-drafts/ and nowhere else. Do not start any night.

## The role

- Live role body: %[2]s
- Role %[3]s, step %[4]s of loop %[5]s
- Frozen step and skills: %[6]s (step.json, skills/)
- Campaign goal:

%[7]s

## Mine the bugs

Search the HK team in Linear for bugs from both sources:

- bugs our QA workers filed (skill linear-bug-report): what the agents
  already catch;
- bugs the assistant coordinator filed for customers and developers:
  what the agents miss.

Keep a bug only when it has all three: a linked feature ticket, usable
repro steps, and an identifiable fix (a merged PR or commit). Record why
each other bug was dropped.

## Write one scenario per kept bug

research-drafts/%[1]s/{main,holdout}/<bug>/ holds:

- scenario.json: {"kind": "role", "llm_judge": true, "checks": [...]}
  with an output-valid check, a bug-caught check whose repro names
  grader/repro.md, and the role's fluff check: for qa-analyst
  {"kind": "fluff", "path": "intake/plan.md", "pattern": "<scenario line
  regexp>", "reasonable": N}, N the scenario count of a reasonable plan
  for the feature; for qa-author {"kind": "code-quality"}. List the feature
  ticket and knowledge pages the agent may read as keyed fake services
  ("key": "id", "responses": {...}); never the bug ticket.
- spec.md: the task spec the coordinator would have written for the
  feature, before the bug was known.
- snapshot/: the code at the pre-fix commit (the fix commit's parent),
  sparse: the files the feature touches plus what the step's skills read
  (playwright/, page objects, seeds, mocks), from both repos when the
  feature spans them. No .git directory, no file from after the fix.
- grader/: repro.md with the repro steps, expected and actual, the bug
  id, and the pre-fix commit, so the bug can later be replayed for real.
  Only the grader reads it.

The format is the eval judge's role scenario; see docs/autoresearch.md
and cmd/spore/testdata/research-roles-e2e/drafts.

## Split

Split by bug into main and holdout: holdout at least 30%% of the bugs and
at least 5 bugs. Bugs of one feature go on the same side.

## Discrimination proof (required)

1. Run the judge on the current role body:
   spore eval judge --kind role --scenarios-root research-drafts/%[1]s
   --frozen %[6]s --candidate %[2]s --set main --out /tmp/current.json
2. Make a weakened copy of the role body (remove its bug-hunting
   instructions) and run the same command on it.
3. The weakened copy must score lower than the current body on the main
   set. Put both scores, and what you removed, in your report.

## Hand-off

The operator reviews the drafts, then approves them with:
spore research judge approve %[1]s --from <your worktree>/research-drafts/%[1]s
`

func (c *researchCmd) judgeDraft(args []string) error {
	pos, err := parse(flag.NewFlagSet("judge draft", flag.ContinueOnError), args, 1, "<campaign>")
	if err != nil {
		return err
	}
	cfg, err := c.store.Load(pos[0])
	if err != nil {
		return err
	}
	goal, err := os.ReadFile(cfg.GoalFile)
	if err != nil {
		return err
	}
	quoted := "> " + strings.ReplaceAll(strings.TrimSpace(string(goal)), "\n", "\n> ")
	tasksDir := "tasks"
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return err
	}
	title := "research judge draft " + cfg.Name
	slug, err := task.Allocate(tasksDir, task.Slugify(title))
	if err != nil {
		return err
	}
	project, _ := task.ProjectName("")
	body := fmt.Sprintf(judgeDraftBody, cfg.Name, cfg.Target, cfg.SkillName, quoted, filepath.Dir(cfg.Target))
	if cfg.Kind == "role" {
		frozen := filepath.Join(c.store.Dir(cfg.Name), "repo", "context")
		body = fmt.Sprintf(roleDraftBody, cfg.Name, cfg.Target, cfg.SkillName, cfg.Step, cfg.Loop, frozen, quoted)
	}
	m := frontmatter.Meta{Status: "draft", Slug: slug, Title: title, Created: time.Now().UTC().Format(time.RFC3339), Project: project}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, []byte(body)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(c.out, slug)
	return nil
}
