---
name: qa-e2e-test-only-changes
description: Use when fixing or hardening {{PROJECT_NAME}} e2e tests (flakiness, waits, selectors, page objects, seeds, fixtures) and you are tempted to edit application product code under src/ to make a test pass or run faster.
---

# QA: E2E fixes stay in test code

Use this when the work is about **Playwright / Cypress e2e**, **flaky tests**, **selectors**,
**waits**, **page objects**, **test data**, or **seeds** - not about shipping product behavior.

## Non-negotiable

- **Do** change: `{{E2E_ROOT}}`, e2e-related Cypress specs/page objects, test fixtures, backend seeds
  that exist **only to support e2e** (via the governance in `editing-backend-seeds-and-mocks`), and CI config for tests.
- **Do not** change **application** code for this class of work: no edits under `src/` (components,
  hooks, product flows) to "make the test pass" or "speed up the test" unless the user **explicitly**
  asks for a product change or you are in a **developer** handoff.
- If the right fix is **in the app** (e.g. drawer close race, a11y, focus), **document it** for devs
  ({{TRACKER}} ticket, PR comment, or a short note) and keep your changes **test-side only** (e.g. wait
  for a stable visible state, narrow locators, `expect.poll`, `toPass`, avoid `waitForTimeout` when a
  condition exists).
- **Minimize workarounds - keep the test on the real user flow.** Test-side != shortcut-side. Fixing a
  test must not mean bypassing the feature: don't swap a real UI action for a synthetic backend
  command / seed transition, and don't assert an end-state the user can't reach. Prefer the existing
  sanctioned mechanism (async -> real action + `{{DRAIN_COMMAND}}` drain + poll; new drain job -> fold
  it into the seed behind `{{DRAIN_COMMAND}}`, not a bespoke seed). See `new-test` rule 11. A shim seed
  that asserts an end state (e.g. repaired badges) without ever clicking the real button that produces
  it is a MAJOR workaround - always use the application from a user's point of view.
- **QA writes e2e only.** If an end state has genuinely no UI surface, do not write a backend unit test
  and do not fake it in the UI; hand it to developers as a coverage note.

## Test-side patterns (prefer these)

- **Overlays / drawers:** wait for the element that blocks interaction to be **hidden** or **detached**
  (e.g. a known `data-testid` used in the POM, or a footer action that only exists while the flow is
  open) - in **page objects**, not in React.
- **Network vs UI:** if GraphQL `waitForResponse` races with the UI closing, fix **listener
  registration order** in the POM (e.g. register the next `query` **after** the mutation response is
  known), or wait for a **user-visible** completion signal after the action.
- **Clicks intercepted:** use the existing pattern for stuck drawers if any (e.g. a reload in
  `validateVisible` is product-adjacent - **don't** add new product hacks; in tests, prefer an explicit
  wait for the overlay to be gone before clicking).

## When product change is required

- Say clearly: *"A durable fix in the app would be X; for QA I only changed tests/POM."*
- Do **not** implement `src/` changes unless the user asks for a developer-style fix.

## Relation to other skills

This skill does **not** replace those workflows; it only constrains **where you edit** for QA e2e fixes.

- **running-e2e-tests** - run or debug a Playwright test (also covers CI file targeting)
- **investigating-e2e-failures** - root-cause a failure from report/trace artifacts
- **starting-the-e2e-backend** - bring up the backend for e2e
- **editing-backend-seeds-and-mocks** - the declare->ack governance +
  authoring rules for any backend seed / {{MOCK_TOOL}} change (seeds live in another repo - never a
  silent cross-repo write)
- **new-test** - scaffold a new Playwright spec

## Where this policy comes from

This is a host-level skill (installed under `{{SKILLS_DIR}}`), not a doc checked into the repo - it is
**not** listed in the repo's root project doc. It is consistent with the repo's own conventions, which
you can cite to a developer:

- The repo's editor test rules (reference: `.cursor/rules/testing.mdc`) -> **"E2E Tests (Playwright)"**
  and its **"No locators in tests"** rule (all selectors live in page objects, tests call POM methods).
- Root project doc -> *"Use Playwright for new E2E tests; Cypress is deprecated and should only be
  touched to fix existing failing tests."*
- `{{E2E_ROOT}}` root doc -> seeds/fixtures/POM conventions (adding `data-testid` to a component is the
  one product-side change the QA role routinely makes - a locator hook, not a behavior change).
