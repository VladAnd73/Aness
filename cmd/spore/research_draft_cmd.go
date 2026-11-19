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
- research-drafts/%[1]s/holdout/<scenario>/scenario.json: 1 to 3 different
  scenarios, sealed until the campaign ends. Never tune them against the
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
	m := frontmatter.Meta{Status: "draft", Slug: slug, Title: title, Created: time.Now().UTC().Format(time.RFC3339), Project: project}
	if err := os.WriteFile(filepath.Join(tasksDir, slug+".md"), frontmatter.Write(m, []byte(body)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(c.out, slug)
	return nil
}
