---
name: sweeping-releases-for-e2e-coverage-gaps
description: Use when asked which recently-shipped {{PROJECT_NAME}} frontend or backend work lacks e2e coverage - "what should we test next", "sweep the last N releases", "any coverage gaps since the last tag", "pick the cheapest specs to add", or when picking QA targets after a release watcher fires. Not for auditing ONE feature's specs (that is reviewing-e2e-coverage).
---

# Sweeping releases for e2e coverage gaps

## Overview

A coverage sweep answers "what shipped recently that no test exercises?" Its only hard part is
**enumeration**: a sweep that misses a feature reports a clean bill of health for work nobody tested.

**Core principle: enumerate from the DIFF. The tracker is for intent, never for inventory.**

## The failure this exists to prevent

A release object in the tracker lists only the issues filed in THAT tracker and linked to it. Work can be
tracked elsewhere (a second tracker, a PR with no ticket, a ticket nobody linked). A release whose tracker
entry reports two issues can ship a complete new feature across several new UI files. A sweep enumerated
from the tracker declares it covered; the feature has zero e2e tests.

If work is dual-tracked across two trackers, a release in one tracker is blind to everything filed only in
the other. Treat every tracker list as partial.

Check it on any release pair:

```bash
git diff --name-status <prev-tag>..<tag> -- src | grep '^A'
git diff --stat        <prev-tag>..<tag> -- src | tail -1
```

The first shows new files the tracker may never list; the second, how much actually changed.

## Procedure

### 1. Freshness gate (do this first, every time)

A sweep off a stale checkout invents gaps and misses fixes.

```bash
{{SECRETS_WRAPPER}} git fetch origin {{DEFAULT_INTEGRATION_BRANCH}} --tags
git log --oneline -1 origin/{{DEFAULT_INTEGRATION_BRANCH}}
git tag --list '{{RELEASE_TAG_PATTERN}}' | sort -V | tail -6
```

Never sweep from a checkout you have not just fetched. State the SHA you swept in the report.

### 2. Enumerate from the diff

For each release pair, the inventory is the diff, restricted to product source:

```bash
git diff --name-status <prev-tag>..<tag> -- src
```

`A` = new surface, `M` = changed behaviour.

- **A new product-source file with no test anywhere is a candidate**, whatever the tracker says.
- **An empty product-source diff means the release shipped no UI** - a legitimate, fast "nothing to do".
  Say so; do not go looking for tickets to justify work.
- Backend releases: same idea against `{{BACKEND_CHECKOUT}}`, but a backend change only becomes a candidate
  when it has a browser-visible consequence (step 4).

### 3. Cross-check two more sources - for INTENT, not inventory

- **The tracker(s)** (`{{TRACKER_TOOLS}}`: the release object, plus any ticket keys in the PR titles): read
  them for acceptance criteria and expected behaviour so a spec asserts the right thing. Never treat their
  issue list as the set of things shipped.
- **{{INTENT_SOURCE}}'s own coverage-hole notes.** Corrective-sync notes in the intent KB often name a
  blind spot before anyone acts on it. Read those notes at the START of a sweep (skill `{{INTENT_SKILL}}`).

### 4. Rule out what has no browser surface - with the reason recorded

| Category | Verdict |
|---|---|
| Mailer templates | No browser surface - not a browser-project candidate |
| Hostname / SSO / brand config (resolved from `window.location.hostname`) | Not assertable in the standard harness |
| Infra / data pipeline | Out of scope |
| {{INTEGRATION_PIPELINE}} / {{VENDOR_B}} sync | Belongs to the `integration` Playwright project + its mocks, not the browser project |
| Performance | Load-test suite, not Playwright |

Record the reason next to each exclusion. An unexplained exclusion is indistinguishable from a miss.

### 5. Rank - cost is a SORT, never a FILTER

Rank survivors by implementation cost (does a page object, seed, or fixture already exist?), then report
them **all**. Ranking by "cheapest = reuses an existing PO/seed" and then cutting the list drops a
brand-new surface with no PO entirely.

A brand-new surface with no page object is the MOST likely to be untested - cost pushes it down the list,
never off it.

### 6. Report

One row per candidate: what shipped (file/PR), where it came from (tag pair), what asserts it today
(nothing / which spec), the acceptance criterion or {{INTENT_SOURCE}} clause a spec would assert, and
estimated cost. Plus: the SHA swept, the tag pairs covered, and every exclusion with its reason.

**File nothing.** A sweep proposes; the operator decides what becomes a task or a ticket. A sweep's
deliverable is the report, not a ticket.

## Quick reference

| Step | Command / source |
|---|---|
| Freshness | fetch `origin {{DEFAULT_INTEGRATION_BRANCH}}`, then sweep - record the SHA |
| Inventory | `git diff --name-status <prev>..<tag> -- src` |
| Intent | {{TRACKER}} release + ticket keys in PR titles + {{INTENT_SOURCE}} |
| Prior blind spots | {{INTENT_SOURCE}} corrective-sync notes |
| Existing coverage | grep `{{E2E_ROOT}}tests/` (and any legacy e2e suite) for the feature's test ids/routes |
| Deep audit of one feature | skill `reviewing-e2e-coverage` |

## Common mistakes

| Mistake | Why it burns you |
|---|---|
| Enumerating from the tracker's release | Work tracked elsewhere is invisible (a two-issue release can hide dozens of changed files) |
| Sweeping a stale checkout | Reports merged work as missing, and misses work merged since |
| Dropping expensive candidates | The most-likely-untested surface is the one with no PO yet |
| Excluding without a recorded reason | Nobody can tell your exclusion from an oversight - including you, next sweep |
| Treating "no product-source diff" as suspicious | It is a real answer; some releases ship only backend or CI |
| Filing tickets from sweep output | Sweeps propose; the operator rules |
