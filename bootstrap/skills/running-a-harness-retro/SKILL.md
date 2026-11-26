---
name: running-a-harness-retro
description: Use when the operator asks for a retro, retrospective or post-mortem on a coding session, or asks why a session went slowly, burned tokens, repeated a mistake, or missed something a check should have caught, and wants changes to the harness (rules, lints, hooks, skills) rather than to the product code.
disable-model-invocation: true
---

# running-a-harness-retro

## Overview

A retro looks at one session and proposes changes to the agent's
**environment** so the next run goes better. The product code is out of
scope. The output is a ranked list of proposals for the operator, each
aimed at a file in the harness.

## Where this stops and the dream run starts

The nightly dream run reads every session from the last night and
writes **memory entries** (facts, preferences, short rules) on its own,
after its reviewer confirms them. A retro does not do that.

| | Retro (this skill) | Dream run |
| --- | --- | --- |
| Started by | The operator, on demand | A timer, nightly |
| Reads | One session, in depth | Digests of every session |
| Writes | Nothing. It proposes. | Memory files, after review |
| Targets | Tracked harness files: rule pool, lints, hooks, skills | The memory directory |

So:

- Never write to the memory directory or to `state.md` from a retro.
- If a finding is a fact or a preference ("this host has no sudo"),
  name it in one line under "Left for the dream run" and move on. The
  dream run will pick it up from the same transcript.
- Do not edit harness files either, unless the operator picks a
  proposal and asks you to build it.

## Steps

1. **Pick the session.** The one the operator names, else the current
   one. Transcripts live under `~/.claude/projects/<encoded-cwd>/` as
   `<session-id>.jsonl`; the encoded cwd is the path with `/` and `.`
   turned into `-`. Never read a transcript end to end: a large one is
   bigger than your context. Grep it for `"is_error":true`, repeated
   commands and the operator's own messages, then read around those.
2. **Read the harness that was live.** Root `CLAUDE.md`/`AGENTS.md`,
   the skills the session loaded, and the project's check command
   (`just check`, `spore lint`, CI workflow). A check that exists but is
   not wired in, or is silently broken, is the finding, not a new one.
3. **Find candidates** in the categories below.
4. **Present them**, most severe first, in the shape under "Report".

## Categories

- **Missing check.** The agent made a mistake a deterministic check
  could have caught: lint, test, type check, hook. A mechanical rule (a
  fixed pattern, a banned call, a file location) gets a check, not
  prose. Target: a new lint in `internal/lints/` (implement the `Lint`
  interface and add it to `Default()` in `lints.go`), a `PreToolUse` or commit-msg hook in
  `internal/hooks/`, or a recipe in `justfile`. A project with no
  guardrail at all (no hook, no CI job running its checks) is a finding
  by itself.
- **Rule that changes nothing.** A line in a steering file the agent
  ignored, or that only restates default behavior. Target: delete or
  rewrite the fragment in `rules/core/<id>.md`, or drop its id from
  `rules/consumers/<consumer>.txt`. Every line in the root files is
  loaded into every session, so a no-op line costs on every turn.
- **Rule in the wrong tier.** Root `CLAUDE.md`/`AGENTS.md` is for
  project-wide rules. Single-area guidance goes in a subdir
  `CLAUDE.md`; rationale goes in `docs/<topic>.md`; a procedure with
  steps goes in a skill. Target: move the fragment and leave a pointer.
- **Navigation.** The agent spent many calls finding a file or a
  fact. Target: a one-line pointer in the source map
  (`rules/core/source-map.md`) or the nearest subdir `CLAUDE.md`.
- **Tool economy.** A tool call was expensive (a huge read, a broad
  fan-out, the same command run many times, a review at a higher
  effort than needed). Target: a skill step that narrows it, a script
  that returns only the signal, or a hook that blocks the wasteful form.
- **Information access.** The agent lacked something it needed: logs,
  a read-only view of a service, a recipe for reaching an outside
  system. Target: a skill under `bootstrap/skills/` with the steps, or
  a `spore` subcommand.
- **Skill that misfired.** A skill fired when it should not have, did
  not fire when it should, or started the agent off badly. Target: the
  skill's `description` (trigger) or its first steps.

## Report

One block per proposal, most severe first:

```markdown
### <n>. <one-line problem>
- Evidence: <session id, timestamp, the call or message, quoted short>
- Cost: <what it cost: turns, tokens, a wrong result, an operator correction>
- Proposal: <the change, in one or two sentences>
- Target: <file path, e.g. internal/lints/<name>.go or rules/core/<id>.md>
- Form: check | rule edit | rule removal | skill | hook | pointer
```

End with:

```markdown
### Left for the dream run
- <fact or preference, one line each, or "none">
```

Write in plain words, imperative voice, ASCII only (see
`rules/core/writing-style.md` in the spore repo).

## Common mistakes

| Mistake | Fix |
| --- | --- |
| Proposing prose for a mechanical rule | Propose the lint or hook; prose that a check could enforce gets ignored. |
| Adding a rule to the root files for a one-area problem | Use a subdir `CLAUDE.md` or a skill. |
| Writing a memory entry "while you are here" | That is the dream run's job. List it under "Left for the dream run". |
| Proposing a new check when one exists but is not wired in | Read the check command first; the wiring is the fix. |
| Reviewing the product code | Out of scope. Retro targets the harness only. |
