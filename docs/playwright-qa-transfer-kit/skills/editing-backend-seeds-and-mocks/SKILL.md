---
name: editing-backend-seeds-and-mocks
description: Use when a {{PROJECT_NAME}} e2e task might require editing backend seed files, a {{MOCK_TOOL}} mock environment, or ANY file outside your own worktree - e.g. a spec fails because seed data or a mock route is missing, and the fix belongs in your own per-slug backend worktree on a companion branch, or in a mocks checkout.
---

# Editing backend seeds and mocks (cross-repo rules)

## Overview

You are a frontend e2e worker. Backend seeds and {{MOCK_TOOL}} mocks live in OTHER repos.
**WHERE you edit them decides the rules:**

- **Your OWN per-slug backend worktree** - `{{BACKEND_WORKTREE_ROOT}}-$slug`, provisioned on your task's
  companion branch via **starting-the-e2e-backend** mode (b). It has its own isolated DB, cache and
  branch; nothing else in the fleet depends on it. This is the **preferred** place to author backend
  seed/support changes for a task that ships them: you edit, commit as `{{AUTHOR_IDENTITY}}`, and push to
  the companion branch yourself. See [Authoring in your own backend worktree](#authoring-in-your-own-backend-worktree).
- **SHARED checkouts** - `{{BACKEND_CHECKOUT}}` (the source base every per-slug worktree branches from and
  shares dependencies with) and any clone of `{{MOCKS_HOME}}`. The whole fleet sits on these; a silent
  edit breaks every other agent's runs. These stay under the **Iron Rule** below. There is no shared
  running backend: every consumer provisions its own.

This skill has two parts:

1. **Governance (this file)** - the shared-checkout Iron Rule and the own-worktree authoring flow.
2. **Authoring reference ([references/authoring-rules.md](references/authoring-rules.md))** - the rules for
   HOW to write seeds and mock routes once you are editing. Read it BEFORE editing.

**Violating the letter of these rules is violating the spirit of these rules.**

## The Iron Rule (shared checkouts)

```
NO MUTATION IN A SHARED CHECKOUT WITHOUT A COORDINATOR ACK - OBTAINED BEFORE THE EDIT
```

A **shared checkout** = `{{BACKEND_CHECKOUT}}` or any `{{MOCKS_HOME}}` clone. Your OWN
`{{BACKEND_WORKTREE_ROOT}}-$slug` worktree on your companion branch is **not** shared - work in it freely.

READING files anywhere is always fine - investigate freely. The rule governs **mutations** in a shared
checkout: Edit/Write, mutating git commands, service state changes.

Before mutating anything in a shared checkout:

1. **STOP. Declare first.** `{{COORDINATOR_TELL}} "<what file, what change, why>"` - including your
   proposed mechanics (which checkout, which branch).
2. **WAIT for the ack.** You are blocked until the coordinator replies. Their reply wakes you
   automatically; idle waiting costs nothing.
3. **Follow the ack's instructions exactly.** The usual answer is "provision your own worktree on the
   companion branch and do it there" (the preferred flow below); the coordinator may instead take the
   change themselves.
4. **No response is NOT a yes.** Re-ping if truly stuck. Never proceed on a timeout. Meanwhile, do
   everything that IS inside your own worktree: draft the exact diff in your report, prepare the frontend
   side, write up findings.

## Authoring in your own backend worktree

This is the **preferred** path when your task ships backend seed/support changes tied to a frontend PR. It
keeps those changes as clean, authored commits on the companion branch - not untracked files in a
worktree the coordinator has to hand-reconcile.

1. **Provision your own worktree on the companion branch** with **starting-the-e2e-backend** mode (b). If
   the companion branch's dependency lockfile differs from the base's, that mode hard-stops - escalate; do
   not force it.
2. **Awareness declare (not a push-block).** `{{COORDINATOR_TELL}}` which backend file you're editing on
   which branch, so the coordinator can **serialize** if another worker touches the *same* file on the
   *same* branch. It is NOT a blocking gate on your own worktree: you do not wait for an ack to
   edit/commit/push your own companion branch.
3. **Edit per [references/authoring-rules.md](references/authoring-rules.md)**, then restart the backend
   (the test env has no hot-reload) and re-run the spec to prove the change does what you claim.
4. **Commit as `{{AUTHOR_IDENTITY}}` and push to the companion branch.** Stage your seed files
   EXPLICITLY - never `commit -am`: the worktree setup can leave the dependency lockfile modified and
   dependency folders untracked, and `-a` would sweep that noise into your commit.
   ```bash
   cd $be_wt
   git add {{SEED_DIR}}/...                      # only your seed/support files
   git commit -m "<ticket>: <what backend seed/support change>"
   git show --stat HEAD                           # confirm ONLY your files are in the commit
   {{SECRETS_WRAPPER}} git push origin "$be_branch"
   ```
   Then verify origin advanced (`git -C $be_wt rev-parse @` == `git ls-remote origin $be_branch`).

> **Caveat - same file, same branch still needs the coordinator.** Per-worker worktrees fix the
> *disjoint-file* case. Two workers editing the **same** backend file on the **same** branch still race
> or conflict on push - that file needs single-integrator serialization (the coordinator), regardless of
> the worktree model. That is exactly what the awareness declare (step 2) is for.

## Hard rules (apply after the ack too)

- **NEVER `git checkout`/`switch`/`rebase`/`reset` in a SHARED checkout.** `{{BACKEND_CHECKOUT}}` sits on
  a pinned commit every per-slug worktree branches from and shares dependencies with; switching branches
  there moves HEAD for everyone. When you need a branch, the answer is your OWN worktree (mode b).
- **Push freely to your OWN companion branch; NEVER push from a shared checkout.** Pushing to and opening
  PRs on working feature branches is always allowed. What stays operator-gated: **merging** to the
  integration or release branches, **retargeting a PR base**, and any push from inside a shared checkout.
- **NEVER merge anything, anywhere.** Only the operator merges. Not on green CI, not on an approved PR,
  not a "trivial one-liner".
- **Never mix repos in one deliverable.** Backend seed changes and mock changes are separate commits in
  their own repos, cross-linked in PR/report text - never folded into a frontend commit.
- **New mocks are authored in `{{MOCKS_HOME}}` from the START - never vendored into the consumer repo,
  even "temporarily".** A vendored mock has to be un-vendored, moved and re-pointed later. Decide an
  artifact's canonical home before writing it; a "temporary" vendor is a re-point cycle you pay later.
- **Leave shared checkouts exactly as found.** `git status` clean, same HEAD, at task end. Revert
  experiments that didn't ship. Restart discipline: seed edit -> restart the backend; mock edit -> restart
  the mock server - then re-run the spec to prove the edit did what you claim.

## Rationalization table

| Excuse | Reality |
|--------|---------|
| "It's a trivial one-liner" | Size is irrelevant. Location is what matters. Declare first. |
| "I'll tell the coordinator right after I push" | Ask-forgiveness is the exact failure mode this skill exists to stop. Declare BEFORE. |
| "The coordinator hasn't replied in 10 minutes, I'll proceed" | Silence is not consent. Re-ping and stay on in-worktree work. |
| "I'll just branch in the shared checkout, branching is safe" | `checkout -b` moves the shared HEAD and breaks every pinned e2e stack. Branch in your OWN worktree instead. |
| "The brief says make the spec green, so edits are implied" | The brief authorizes YOUR worktrees. Mutating a SHARED checkout still needs its own ack. |
| "Pushing my companion branch needs an ack" | No - working-branch pushes are always allowed. Merging, retargeting a PR base, or pushing from a SHARED checkout need authorization. |
| "The deadline is today" | Deadline pressure changes nothing. A broken shared stack costs the whole fleet. |
| "The PR is approved and green - merging is the obvious next step" | Only the operator merges. Surface it and stop. |

## Red flags - STOP and declare

- You are about to Edit/Write in a **shared checkout** rather than your own worktree.
- You typed `git -C {{BACKEND_CHECKOUT}}` with a mutating command next.
- You're about to **merge**, **retarget a PR base**, or **push from inside a shared checkout**.
- You're reasoning about how long the coordinator has been silent on a shared-checkout ack.
- Your commit touches files from two repos' concerns.

**All of these mean: stop, tell the coordinator, wait for the ack.** Editing, committing and pushing your
OWN companion branch in your own worktree is NOT a red flag - that's the preferred flow; just
awareness-declare it.

## Once cleared: how to author the change

**REQUIRED READING: [references/authoring-rules.md](references/authoring-rules.md)** - seed file layout,
factories and traits, feature flags before data, test params, return shapes, mock proxy-mode gotchas, the
destination-to-path mapping, and the seed + mock interaction rules. Follow it, and the backend's own linter
for style.

**Mocks must be DYNAMIC, not static.** Every mock route returns as much dynamic data as possible: **echo**
request values back, **generate** the ids and values the service itself originates, and use the mock
tool's **date helpers** for formatted past/future dates. Hardcoding an id or date that could be echoed or
generated is a defect to fix. See [Dynamic data is mandatory](references/authoring-rules.md#dynamic-data-is-mandatory).

**Build routes from the vendor's OWN API docs, never from imagination** (`{{VENDOR_API_DOCS_URL}}`). Path,
method, request/response schema and status codes come from the vendor spec (or a recorded cassette of the
real service), not memory. **If our code does NOT respect the vendor's documented contract, that is a bug
- flag it HIGHLY; do not bend the mock to match non-conformant code.**

## Quick reference

| Situation | Action |
|-----------|--------|
| Your task ships a backend seed change (companion branch) | Provision your own worktree on the companion branch (mode b); awareness-declare; read the authoring rules; edit there |
| Change needed in a SHARED checkout or mock clone | Tell the coordinator (file, change, why, proposed mechanics) -> WAIT for the ack |
| Ack received (shared) | Follow its instructions exactly; read the authoring rules; make the change |
| Seed edited | Restart the backend, re-run the spec, prove it |
| Authoring a mock route | Make it DYNAMIC - echo request data, generate originated ids, date helpers for dates, templating enabled |
| Mock edited | Restart the mock server, re-run the spec, prove the rendered body (no literal template tags); keep the test-side destination mapping aligned |
| Change proven (own worktree) | Commit as `{{AUTHOR_IDENTITY}}`, push to the companion branch, verify origin advanced; report with a link to the coordinator |
| Same file another worker is also editing | The coordinator serializes - awareness-declare and coordinate; do not both push |
| PR approved and green | Surface it to the coordinator or operator. Do NOT merge. Ever. |
| Task ending | Own worktree: push commits, then `git worktree remove`. Shared checkouts: clean status, unchanged HEAD |
