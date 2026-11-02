---
name: testing-a-feature-end-to-end
description: Use when taking a {{PROJECT_NAME}} feature from a ticket all the way to tested - the operator says "test this feature", "take {{TRACKER_PROJECT_KEY}}-123 end to end", "QA this feature", or hands a feature-definition {{TRACKER}} ticket or {{CHAT_TOOL}} thread and wants the full arc from analysis to e2e specs to a reviewed PR to a bug report.
---

# Testing a feature end to end

## Overview

One orchestrator that takes a {{PROJECT_NAME}} **feature-definition ticket** all the way to a tested,
reviewed PR plus a bug report. It does **not** re-teach how to run e2e, analyze a ticket, or file a bug: it
**sequences and gates** the existing granular skills, in order, and carries the lessons that only show up
when you run the whole arc.

**Core principle:** intended behavior ({{INTENT_SOURCE}} / docs) is the oracle; the real UI is the subject;
every claim is verified against primary evidence; a found bug is a SUCCESS to surface, never something to
green-match away; and nothing crosses a [GATE] without the operator.

- **Entry:** a {{TRACKER}} ticket or a feature description / {{CHAT_TOOL}} thread.
- **Exit:** an e2e PR *opened, never merged* that has passed a code review and a senior-QA
  coverage/assertion-depth review, plus one final report of found + proposed bugs and all review results.
  Verified bugs are **filed directly** (no operator-approval gate on filing).

**This skill loads its detail on demand - read the reference files as you reach each stage:**
- `references/pipeline-lessons.md` - per-stage rules, including the anti-lessons (mistakes that recur
  when the whole arc is run). Read the relevant stage section BEFORE doing that stage.
- `references/recipes.md` - infra recipes (per-slug backend, the review-app sign-in flag, the vendor mock,
  pushing under the secrets wrapper, schema load, the send-keys dispatch recipe).

## Execution model (operator-locked)

- The coordinator dispatches **one spore worker** that runs the whole pipeline **sequentially**.
- That worker spawns **subagents only for the independent review passes** - the senior-QA
  coverage/assertion review (agent `senior-qa-e2e-reviewer` / skill `reviewing-e2e-coverage`), the PR code
  review, and the senior-QA validation of found bugs. Reviews are independent so they do not inherit the
  author's blind spots.
- Large features MAY fan spec authoring out to file-disjoint subagents **partitioned by page-object
  ownership** (one sole editor per PO file); cross-PO tests are held as a coordinator phase 2. Default is
  sequential.
- Backend: the worker boots its OWN isolated per-slug backend worktree
  (`{{BACKEND_WORKTREE_ROOT}}-{{SLUG}}`, own DB/cache/port). **There is no shared e2e backend** - every
  consumer provisions its own; nothing is shared to mutate. (Skill `starting-the-e2e-backend`, branch-aware
  mode.)

## Run-state checkpoint (survives respawns)

Long pipelines outlive a worker's token budget; on the token threshold the worker is killed and the harness
respawns it while the task is active. To resume without redoing work:

- The worker maintains a **plan + status checkpoint** at
  `{{HOST_STATE_DIR}}/feature-qa-runs/<ticket-or-slug>-run-state.md`.
- The dispatch brief (the task file, which persists across respawns) **names this path** and instructs:
  "On (re)start, read this file FIRST and resume from the current stage; do not redo completed stages."
- Contents (overwrite by section; must not grow unbounded):
  1. **Plan** - the tailored pipeline for THIS feature (which stages apply, scope).
  2. **Stage status** - per stage: done / in-progress / blocked / pending, with the exact next step.
  3. **Decisions locked** - operator gates already answered; scope calls; mock/no-mock decisions.
  4. **Artifacts** - branch, PR link, latest SHA, backend worktree path/ports, seed/mock edits declared,
     subagent verdicts collected so far.
  5. **Open gates** - what is currently waiting on the operator, so a respawn re-surfaces it and never
     silently proceeds past a gate.
- Update it **after every stage transition and before idling.**

## The pipeline ([GATE] = operator gate)

Read the matching stage in `references/pipeline-lessons.md` before running each one.

| # | Stage | Reuses | Key gate / rule |
|---|-------|--------|-----------------|
| 1 | Intake / ticket analysis | `analyzing-test-tickets` | Developer tickets STRICTLY read-only; verify WHERE the specs actually live (remote PR tip, not the integration branch) |
| 2 | Product intent | `{{INTENT_SKILL}}` | Docs first; intended behavior = the bug-hunt oracle; every intent claim gets a verified link |
| 3 | Test planning & scope | (glue) | Consolidate (no standalone page-load tests; merge same-seed/setup) down to the **safe floor**, zero coverage loss; full CRUD; compute the FULL expected effect-set per action up front |
| 4 | Backend + seed + mock | `starting-the-e2e-backend`, `refreshing-the-backend-checkout` (not included in this kit), `editing-backend-seeds-and-mocks` | Own per-slug backend; additive flag-gated seeds; [GATE] for schema/factory/app-code edits; shared mocks-repo edits go through the coordinator (declare -> ack) - additive, pattern-following routes are coordinator self-ack, anything structural is still [GATE] |
| 5 | E2e authoring | `new-test` + `running-e2e-tests` | Playwright, Page Object Model, **FLAT BAN on comments** in test code (see below) |
| 6 | Assertion depth | Assertion playbook below | Assert ALL effects (state change + every outbound vendor request via the mock transaction-log helper), not banners alone |
| 7 | Run / CI + failure triage | `investigating-e2e-failures` | Drive the REAL UI (no synchronous job shims); re-verify raw evidence yourself; run the FULL folder, not one file |
| 8 | Bug hunting | intent-vs-observed gap | A found bug is a SUCCESS -> `test.fixme` + finding; NEVER green-match; NEVER edit product source (`qa-e2e-test-only-changes`) |
| 9 | Open PR | (git) | Push to the working branch (always OK); **NEVER merge / never retarget base** [GATE]; author `{{AUTHOR_IDENTITY}}` |
| 10 | Quality loop | `reviewing-e2e-coverage` / `senior-qa-e2e-reviewer` | (a) code-review the diff -> **fix**; (b) re-run coverage/assertion review post-fix; (c) senior QA adversarially validates the found bugs |
| 11 | Interaction & branch completeness check | `senior-qa-e2e-reviewer` (handback target) | INDEPENDENT worker enumerates the FULL interaction surface - every UI element's full designed interaction set (not click-only), or every field/param of a changed API/GraphQL endpoint if there is no UI - plus every feature-relevant code branch; anything unexercised by a test OR a recorded manual check is a gap; important gaps go back to senior QA for a real test, minor ones may close via a documented manual-check note |
| 12 | Final report | (glue) | One report: found + proposed bugs + all review results, to the operator |
| 13 | Bug reporting | `{{BUG_FILING_SKILL}}` | File verified bugs directly; duplicate search + format pass still mandatory; developer tickets read-only |

### Comments in test code (stage 5)

HARD RULE: add NO comments to spec, page-object, fixture, or seed files. Rename or restructure instead.
- Provenance (which ticket, which commit, which backend file) goes in the PR description or the ticket.
- Conventions go in the folder's `CLAUDE.md` (shipped in this kit as `CLAUDE.tmpl.md`).
- No exceptions: not a user-behaviour note, not a ticket link.
- A comment-vs-comment merge conflict is resolved by deleting the comment.

## Assertion-depth playbook (stage 6)

- For each action, enumerate the **FULL expected effect-set BEFORE writing asserts** - that set is the
  deterministic completion oracle (know beforehand exactly what the action will cause, assert ALL of it,
  then the test is done).
- Assert the PRIMARY state change AND every downstream effect: the outbound {{VENDOR_A}} request (and any
  call to an external estimation service) captured via the reusable mock transaction-log helper (reference
  project: `playwright/support/vendorAMock.ts`); DB/state transitions; list content.
- Grade each assertion: **[Presence-only] / [Shallow] / [Deep].** Banners/dialogs are SECONDARY, never
  sufficient alone. "Check the button exists" is the anti-pattern - click it and assert the outcome.
- Match by **stable identity** (e.g. the item's name), not a shared/reused uuid that async fan-out
  contaminates.
- Cover full CRUD; exercise flows with outcomes asserted, not button presence.
- For an async fan-out, the completion oracle is "all expected effects observed" - inspect the
  executor/queue state (drain until empty, RAISE on timeout), never a blind sleep.

## Interaction & branch completeness playbook (stage 11)

An INDEPENDENT worker (never the spec author) enumerates the FULL interaction surface the feature exposes,
then checks each item was exercised at least once - by an automated test OR an explicitly recorded
manual-check note. Three axes:

1. **UI interaction completeness - every element's full designed interaction set, not click-only.** Read
   the component's own handlers (`onClick`, `onChange`, `onDrag*`, a functional `onMouseEnter` /
   hover-reveal, `onKeyDown`, ARIA `role`) to find what it was actually built to support - click,
   type/fill, select, drag, hover-reveal, checkbox/toggle, keyboard activation, right-click/context menu,
   multi-select - and cross-check each is the target of a matching Playwright action somewhere in the suite.
2. **API/GraphQL field completeness - for a ticket with no UI surface (or a mixed one).** Enumerate every
   request field (required AND optional), every response field, and every distinct error/status path a
   changed endpoint or mutation can take - not just "the endpoint was called once."
3. **Code-branch coverage** - using the repo's Istanbul wiring (`vite-plugin-istanbul` +
   `VITE_COVERAGE=true` + `.nyc_output` + `nyc`), scoped to only the feature's changed files.

**Exhaustive enumeration != exhaustive automation.** The enumeration itself must cover everything;
automation does not have to. For each gap, judge importance: state-changing / correctness-critical /
regression-prone -> hand back to senior QA to author a real automated test. Purely cosmetic or low-risk gaps
may close instead with a documented "manually verified: what + how" note - but the note must exist; an item
with neither a test nor a note is still an open gap.

Full mechanism, gotchas, and known blind-spot categories are in `references/pipeline-lessons.md` section
11 - read it before running this stage.

## Hard rules & gates (do not cross without the operator)

- A verified bug is filed directly - no operator-approval gate; the duplicate search and format pass are
  still mandatory.
- Developer tickets are STRICTLY read-only (no edits, no comments); associate only via a relation link from
  an operator-owned QA ticket.
- Agents NEVER merge; only the operator merges. Push to the working branch is always OK (push != merge).
- Surface bugs, never green-match; never edit product source to make a test pass (`qa-e2e-test-only-changes`).
- Assert all effects, not just the banner.
- Drive the app from the real user's point of view; minimize workarounds.
- Verify and link every claim; re-verify raw evidence yourself, never trust a second-hand parse.
- Flat ban on comments in test code; consolidate tests.
- Cross-repo edits to SHARED checkouts follow declare -> ack; the worker's OWN per-slug backend worktree is
  not "shared".
- Once the operator rejects or holds a decision, never re-raise it (especially not via a question prompt).

## Red flags - STOP

- About to make a failing test green by matching the app's buggy behavior -> it is a bug; `test.fixme` +
  surface it.
- About to edit product source to fix a test -> STOP; keep the fix in test code, document the app fix for
  developers.
- About to add a synthetic backend shim (run a job synchronously, a fake route) instead of clicking the real
  button -> workaround; drive the real UI.
- About to assert only a banner/toast -> shallow; assert the state change and the outbound request too.
- About to hand a static root cause to the operator as if reproduced -> say it is static, or reproduce it in
  isolation.
- About to trust a review's or another agent's claim ("file absent", "N -> M tests", "the path makes zero
  outbound calls") -> verify against the live tree / raw trace first.
- About to merge or retarget a PR base -> [GATE] operator-only.
- About to proceed past a [GATE] with no operator answer -> stop and surface it.

## What this skill must NOT do

- Duplicate the granular skills' content - it SEQUENCES and gates them, citing each by name.
- Invent mechanisms - every step references a verified skill or is explicitly new glue.
- Proceed past a [GATE] autonomously.
