**Status**: design approved by the operator 2026-11-16; next: implementation plan

# Configurable task loops

## Why

A long skill such as `testing-a-marketer-feature-end-to-end` (13 stages)
is executed by one worker that has to remember the order, keep its own
progress file, decide when to start "independent" reviewers, and decide
when a stage is finished. Every one of those is a judgement call an
agent can get wrong. The operator wants that sequencing moved out of the
agent and into spore: a driver that reads a task's files, sees which
step the task is on, and starts the next step, with each step run by an
independent agent.

Upstream (the team repository's `main`) already has such a driver,
`DriveRoleLoop`, but its step order is fixed in Go: engineer, reviewer
A, reviewer B. That shape suits feature development; it does not fit
QA. The operator's ask: keep upstream's machinery, make the steps
configurable through loop files.

## Goals

- A loop file defines a loop: its steps, what runs at each step, what
  counts as done, and where the loop goes next (including back to an
  earlier step).
- A task opts into a loop by name in its frontmatter. One driver runs
  any loop on each fleet reconcile pass.
- Two loops ship:
  - `dev`: reproduces upstream's engineer, reviewer A, reviewer B loop
    exactly, as a loop file.
  - `qa-e2e`: the stages of `testing-a-marketer-feature-end-to-end`,
    each stage an independent agent (stages may be grouped into one
    step where they share one agent's context, see "QA loop").
- Reuse as much upstream code as possible: pane spawn and wake with a
  verified send, liveness and respawn cap, stall detection, done and
  escalation markers, coordinator notifications, the GitHub draft-PR
  mirror.
- Our fork's spawn hardening also covers loop panes: spawn backoff
  (`fleet-spawns.json`), agentenv seeding, network isolation, the inbox
  watchdog.

## Non-goals

- No change to tasks without a `loop:` key: they keep today's single
  worker, byte-identical.
- No "wait for operator" step type. The operator retired the QA locks on
  2026-11-16 (see "Rules carried into the QA loop"); anything a loop
  cannot settle escalates to the coordinator, like a round cap.
- No mixing QA steps into the dev loop now. Loop files make it possible
  later; that is a future use, not this work.
- No merge by any agent or by the driver. Ever.

## Anti-goals (would make this a failure even if it works)

- A loop that advances on an agent's say-so where a mechanical check was
  cheap. The point is determinism.
- Forking upstream's loop so far that it cannot be offered back
  upstream or re-synced later.
- Breaking the existing single-worker fleet in any way.

## Constraints

- Go, inside `internal/fleet` and `internal/task` of this fork. Our
  fork and upstream diverged since `5867ec5` (2026-06-08); 21 files in
  those packages changed on both sides, hottest `internal/fleet/fleet.go`
  (our spawn backoff sits in the same reconcile loop where upstream
  inserts the role-loop call).
- Upstream code to port (upstream/main): `internal/fleet/role*.go`,
  `internal/fleet/reviewers_config.go`, `internal/task/role{file,task}.go`,
  `bootstrap/roles/*.md`, `docs/role-loop.md`, plus `internal/editor`
  (`fleet.go` calls `editor.RefreshWorkspace`) and `internal/lang`.
  About 3.2k lines of code and 3.7k lines of tests.
- Acceptance-first TDD per CLAUDE.md: every acceptance scenario below is
  a test, the whole set proven red on assertions before implementation.
- Opensource-bound: no host paths or internal names in shipped files.

## Approaches considered

- **A. Port upstream's loop, make the step order configurable.**
  Picked by the operator. Keeps upstream's design and most of its code;
  the configurable step order can be offered back upstream.
- B. Write a small driver of our own, copying only upstream's pane
  helpers. Less code, but drifts further from upstream.
- C. Chain ordinary tasks with `--needs`. Nearly free, but cannot go
  back a step and has no conditions or round caps.

## Design

### Phase 1: port as-is

Port upstream's role loop unchanged (`loop: role`), with its tests
passing in our tree, and wire our spawn hardening into
`rolespawn.go` / `roleliveness.go`. This proves the machinery on our
fork before anything changes, and gives the dev-loop parity baseline.

### Phase 2: loop files

A loop file is TOML (matches `spore.toml`). Shipped loops are embedded
in the binary; a project file at `bootstrap/loops/<name>.toml`
overrides a shipped loop of the same name, the same rule upstream uses
for `bootstrap/roles/<role>.md`.

A task selects a loop with `loop: <name>` in its frontmatter. Upstream's
`loop: role` stays valid and means the `dev` loop.

A loop file has:

- `steps`: an ordered list. Each step has:
  - `name`
  - `kind`: `agent` (a pane running a role body) or `action` (a
    built-in spore operation, run by the driver, no agent).
  - for `agent`: `role` (a role body name resolved like upstream's role
    bodies), `session` (pane name; two steps that name the same session
    share one living agent, as upstream's engineer persists across
    rounds), optional `skills` to name in the role body's wake message.
  - `done`: the conditions that must ALL hold before the step counts as
    finished. Condition kinds:
    - `file`: a path under `.spore/<slug>/` exists and parses (JSON
      schema per artifact, as upstream's bounce logic validates today).
    - `verdict`: a verdict file's field equals a value (`approve`,
      `request_changes`).
    - `command`: a command run by the driver in the task worktree exits 0
      (for example `just test`, or a check that a report has its
      required sections). Bounded by a timeout.
  - `next`: transitions. Default is the following step. A step may map
    a verdict value to a step name (`request_changes -> engineer-revise`).
  - `max_rounds`: round cap for a step that can be re-entered; hitting it
    escalates.
- Built-in actions for v1: `push-and-open-pr` (push the task branch and
  open a PR as the GitHub App bot, `QA` label except in the
  `infrastructure` repo, assignee `operator_login`, per the operator's
  2026-11-13 ruling and fork PR #63), and `notify-coordinator`.

The driver derives the current step only from files on disk, exactly as
upstream's `DeriveSnapshot` does, so every tick is idempotent and a
respawned agent or coordinator resumes in the right place. Upstream's
`derivePhase` becomes an interpreter over the loop file; its phase names
for the `dev` loop stay the same so upstream's tests keep passing.

### The `dev` loop

A loop file that encodes upstream's machine exactly:
engineer-initial, review-A, engineer-revise-A, review-B,
engineer-revise-B, done or escalated, `max_rounds = 3` per reviewer,
reviewer A killed when B starts, engineer session persisting. Upstream's
role-loop tests run against it unchanged as the parity proof.

### The QA loop (`qa-e2e`)

Steps follow the skill's pipeline. Stages whose work is one continuous
authoring effort share one agent session; the reviews are separate
agents, which is the point:

| Step | Skill stages | Agent |
|---|---|---|
| intake | 1 analysis, 2 product intent, 3 test plan | analyst |
| author | 4 backend/seed/mock, 5 specs, 6 assertion depth, 7 run/triage, 8 bug hunt | author (persists) |
| open-pr | 9 | action `push-and-open-pr` |
| review | 10 code review + coverage review + senior-QA bug validation | reviewer (fresh) |
| completeness | 11 interaction and branch completeness | completeness reviewer (fresh) |
| report | 12 final report, 13 file verified bugs | reporter |

A `request_changes` from review or completeness returns to `author`,
capped. The exact grouping is a builder call to confirm against the
skill's `references/pipeline-lessons.md`; the rule is "separate agent
wherever independence matters".

Role bodies for the QA steps live with the marketer skills (not in this
open-source repo): the loop file names roles, the project supplies
`bootstrap/roles/<role>.md`. The loop file itself is project config in
`marketer-frontend`.

### Rules carried into the QA loop (operator, 2026-11-16)

- QA never changes the backend's schema, factories or app code; only
  additive flag-gated seeds. If a ticket cannot be tested without it,
  that is a finding.
- Mocks-repo edits, structural included, need no operator question
  (the shared-checkout coordinator ack still applies).
- No PR is merged before its reviews are done; only the operator merges.

These are already in the skill; the QA role bodies inherit them.

## Acceptance scenarios

1. Given a task without `loop:`, when reconcile runs, then a single
   worker is spawned exactly as today.
2. Given a task with `loop: role`, when the driver ticks through
   upstream's file sequence, then phases match upstream's role-loop
   tests exactly (ported suite green).
3. Given `loop: dev`, same as 2 (the `dev` loop file is the parity
   proof).
4. Given a three-step test loop (agent, agent, action) with fake agents
   that write their done files, when the driver ticks, then each step
   starts only after the previous step's `done` conditions all hold, and
   the action runs once.
5. Given a step whose `done` has a `command` condition that exits 1,
   when its file exists, then the loop does not advance and the
   coordinator is told once.
6. Given a `request_changes` verdict with `next` back to an earlier
   step, when the cap is not reached, then the earlier step's session is
   woken with the verdict path; at the cap, the task escalates.
7. Given a project override `bootstrap/loops/<name>.toml`, when the
   driver loads that loop, then the override wins over the embedded one.
8. Given an invalid loop file (unknown step in `next`, unknown kind),
   when the task is activated, then the driver refuses with a clear error
   and spawns nothing.
9. Given a loop pane that dies repeatedly, when spawned, then our spawn
   backoff and the respawn cap both apply (no spawn storm).
10. End to end: given the `qa-e2e` loop file and stub role bodies whose
    agents write their files, when a task runs through reconcile ticks
    against a fake GitHub (reuse `cmd/spore/gh_e2e_test.go`'s fake),
    then it reaches done, opened exactly one PR as the bot with `QA`
    and the assignee, and never merged.

## Calls made after the builder check (coordinator, for operator review)

1. **Going back and returning.** A `next` entry can send work back to an
   earlier step and mark it `return = true`: once that step's `done`
   holds again, the loop returns to the step that sent it back (not the
   step after it). This is upstream's revise semantics
   (engineer-revise-A goes back to review-A), generalised. The current
   step is derived from round counts per step on disk, as upstream's
   `derivePhase` does.
2. **Where things live.** spore ships the engine, the `dev` loop, and a
   small generic example loop. `qa-e2e`, its role bodies and any QA
   wording live in `marketer-frontend` (`bootstrap/loops/`,
   `bootstrap/roles/`), not in this open-source repo. Scenario 10 runs a
   stand-in loop with the same shape (agent, agent, action, two
   reviewers, report) in spore's tests.
3. **Actions are safe to repeat.** `push-and-open-pr` opens the PR on
   its first run and only pushes on later visits; it records the PR in
   `.spore/<slug>/github.json` (upstream's mirror file).
4. **A failing `command` check.** It runs once per change of the step's
   files (keyed by a fingerprint, like upstream's summary
   fingerprint), never on every tick. On failure the driver wakes the
   step's agent with the command output. It does NOT count toward
   `max_rounds`: a failed check only means that agent's work is not
   done yet (operator, 2026-11-16).
5. **Reviewer modes.** An `agent` step may set `mode` (`fleet` or
   `code-review`), `agent` and `effort`, generalising upstream's
   `[fleet.reviewers.A/B]` slots. The `dev` loop reads the existing
   slot config so upstream's settings keep working unchanged.
6. **Step file shapes.** Upstream's engineer-response and verdict
   shapes are built in. A step may also declare its own file path and a
   list of required JSON fields; nothing richer than that in v1.
7. **Bot PR details.** Fork PR #63 (assignee) is merged, so the
   action builds on it. The action's label and assignee come from config
   (`label`, `operator_login`), so upstream is not forced to use our
   `QA` label. A fake GitHub covering PR create, label and assignee
   calls is in scope for scenario 10.

## Left to the builder

- The exact TOML schema and field names, within the shape above.
- Where the interpreter lives (`roleloop.go` refactor vs a new
  `loopdef.go`), as long as upstream's tests still pass.
- How much of `rolegithub.go` (the draft-PR mirror) is wired in v1
  versus left off by default as upstream ships it.
- The QA step grouping details.

## Process

Plan via `superpowers:writing-plans`; the plan's first task writes all
acceptance tests and proves them red. Execution via
`superpowers:subagent-driven-development`; every worker brief names
`superpowers:test-driven-development` and the built-in `code-review` at
`low`. `just check` green before any push. Ships as its own fork PR, then its own upstream PR (opened as the bot, operator approves); fork PR #63 and upstream #109 merge separately as they are (operator, 2026-11-16).
