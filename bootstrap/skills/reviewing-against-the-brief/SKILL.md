---
name: reviewing-against-the-brief
description: Use when a spore worker is about to report DONE on a task, or a coordinator is checking a worker's branch before merge, and the question is whether the diff delivers what the task brief in tasks/<slug>.md asked for - missing acceptance scenarios, scope creep, a "Do NOT produce" item that slipped in, or a project rule the diff breaks that no lint catches.
---

# reviewing-against-the-brief

## Overview

Two separate reviews of one diff, reported side by side:

- **Pass 1, brief**: does the diff do what `tasks/<slug>.md` asked for?
- **Pass 2, project rules**: does the diff follow the rules this
  project wrote down, where no lint already enforces them?

A change can pass one and fail the other. Code that follows every rule
but skips an acceptance scenario fails pass 1. Code that meets every
scenario but adds a `Co-Authored-By` trailer the rules ban fails pass 2.
Keep the two reports apart so one cannot hide the other.

This skill does not hunt for correctness bugs or cleanups. The built-in
`code-review` and `simplify` skills do that. Run them too if the brief
asks for them; do not fold their findings into this report.

## 1. Pin the diff

The fixed point is what the caller named. If nobody named one, use
`main`: a worker branch is cut from it.

```bash
git rev-parse --verify <fixed-point>          # fails on a bad ref
git diff --stat <fixed-point>...HEAD          # three dots: from the merge-base
git log --oneline <fixed-point>..HEAD
```

Stop here if the ref does not resolve or the diff is empty. Say so in
the report. Do not send an empty diff to two sub-agents.

## 2. Find the spec: the task brief

The spec is the worker brief `tasks/<slug>.md`. Find the slug in this
order:

1. The caller named it.
2. `$SPORE_TASK_SLUG` is set (you are inside a worker session).
3. The branch is `wt/<slug>`.

Read the brief from the main checkout, not the worktree: the worktree
copy can be stale. `git rev-parse --git-common-dir` gives you the main
repo's `.git`; its parent holds `tasks/`.

Pull these parts out of the brief. Each is a list pass 1 checks:

- **Acceptance scenarios**: the "given X, when Y, then Z" lines. Some
  briefs use a `## Acceptance scenarios` heading; older ones use a
  bold or inline "Acceptance scenarios" lead-in followed by a numbered
  list. Every scenario is a requirement.
- **Deliverable**: the files, commits and report the brief names.
- **Do NOT produce**: each item is a prohibition the diff must respect.
- Any explicit "update X" line outside the scenarios (docs, usage text,
  CHANGELOG).

If there is no brief, skip pass 1 and write "no brief found" under its
heading. Do not invent a spec from the commit messages.

## 3. Find the project rules

Pass 2 reads the rules this project wrote down:

- Root `CLAUDE.md` and `AGENTS.md`, plus any subdir `CLAUDE.md` on a
  path the diff touches.
- In the spore repo itself, the root files are rendered from the rule
  pool under `rules/core/`. Cite the `rules/core/<id>.md` file, since
  that is where a fix to the rule would land.

Then drop every rule a lint already enforces. Run `spore lint` once
and note what it covers (em-dash, file size, comment noise, decoration,
drift). A finding a lint would catch is noise in this pass; the lint
reports it anyway.

What is left are judgment rules. In a spore project these are typical:

- Acceptance tests were written before the code (red, then green).
- A comment adds something the code does not already say.
- Commit messages carry no `Co-Authored-By` or "Generated with" lines.
- Multi-session specs go under `docs/todo/`; a plan file is not tracked.
- Writing style: plain words, imperative voice, no emojis.

## 4. Run both passes in parallel

Send both prompts in one message so the sub-agents run at the same
time, each with a fresh context. Each sub-agent gets the diff command,
the commit list, and nothing from the other pass.

**Pass 1 prompt** includes the full brief text (or its path) and this
ask:

> For each acceptance scenario, report one status: met (name the test
> and the commit), partial, missing, or deviated (the diff does
> something else on purpose, and the brief or commit message says why;
> the coordinator must sign it off). Then list (a) "Do NOT produce"
> items the diff violates, (b) behavior the diff adds that the brief
> did not ask for, (c) tests that cannot catch the problem they name:
> for a "something happens" scenario, the test would pass without the
> change; for a "nothing changes" scenario, the test would still pass
> if the change leaked into that case. Mark deliverables a diff cannot
> show (a pushed branch, a tell, a report file) "not checkable from the
> diff". Quote the brief line for every finding. Under 400 words.

**Pass 2 prompt** includes the list of rule files and the judgment
rules you kept in step 3, and this ask:

> Report each place the diff breaks one of these rules. Cite the rule
> (file and the sentence). Mark each finding hard (the rule is
> explicit and the diff plainly breaks it) or judgment. Skip anything a
> lint enforces, and anything the brief itself states: pass 1 owns
> those. Under 400 words.

## 5. Report

```markdown
## Brief (tasks/<slug>.md)
<pass 1 report, lightly cleaned>

## Project rules
<pass 2 report, lightly cleaned>

Brief: <n> findings, worst: <one line or "none">.
Rules: <n> findings, worst: <one line or "none">.
```

A finding is a scenario that is not met, or one item under (a), (b),
(c) or pass 2. Met scenarios and "not checkable" deliverables are not
findings.

Do not merge the two lists or rank across them. A missing acceptance
scenario and a broken style rule are different kinds of problem, and
the reader decides which matters.

A headless worker does not wait for anyone to read this. Fix what you
can, then put the report (or its path) in your DONE tell.

## Common mistakes

| Mistake | Fix |
| --- | --- |
| Counting a scenario as met because a test with a similar name exists | Read the test. It must fail without the change. |
| Reading the brief from the worktree | Read it from the main checkout; the coordinator may have edited it. |
| Repeating lint findings in pass 2 | Run `spore lint`; drop what it covers. |
| Reporting correctness bugs here | Those belong to the built-in `code-review`. |
| Treating "no brief" as "all met" | Write "no brief found" and stop pass 1. |
