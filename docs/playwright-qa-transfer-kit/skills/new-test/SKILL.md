---
name: new-test
description: Use when scaffolding a new Playwright e2e spec for {{PROJECT_NAME}}, or refactoring/reviewing an existing spec for quality - you need a spec that follows the suite's conventions, and you are unsure which seed, fixture, page object or domain folder to use, how to assert the end state, whether to add a comment, whether to merge tests that share a setup, or how to prove a third-party call happened.
---

# New Playwright Test Scaffolder

You are scaffolding a new Playwright e2e test for {{FRONTEND_REPO}}. Playwright is the active e2e suite;
Cypress is legacy (maintenance only - don't scaffold new Cypress specs).

## Conventions live in the repo - defer to them, don't re-derive

The repo already documents test structure, seeds, tags, and transitions. **Read the canonical source
that fits the area you're testing instead of re-deriving it here** (the same way
`investigating-e2e-failures` defers to the suite root doc):

- **`{{E2E_ROOT}}` root doc** (the folder's `CLAUDE.md`) - the primary reference for Playwright specs:
  "How Seeds Work" (`server.seed('clean')` + `userFlows.loginWith()` auto fixture), "Standard test
  structure" (`import { test } from '~/playwright/support/fixtures'`, `describe`/`beforeEach`/`seed`/
  `changeTenantTo`/`goto`), "Tags" (`@smoke`), "Item state transitions (via server)"
  (`takeItemToStatus`/`waitForItemStatus`), key file paths, and the backend seeds map.
- **The repo's editor test rules** (reference: `.cursor/rules/testing.mdc`) - **"E2E Tests
  (Playwright)"** (incl. "No locators in tests", "Page Objects", "Test Patterns"), **"Test Data"**
  (factories/fixtures, cleanup), and - for the Cypress side or non-e2e work - its "E2E Tests (Cypress)"
  and "Unit Tests (Vitest)" sections.
- **Root project doc** - repo-wide testing policy ("Use Playwright for new E2E tests; Cypress is
  deprecated"), commands, and the "Testing Rules" summary.
- **`editing-backend-seeds-and-mocks`** - how backend seeds are authored + the
  declare->ack governance for editing them (they live in another repo).

Do **not** repeat those conventions inline and risk drift - cite them. This skill adds what they
don't: the **interactive intake**, the **investigate-before-scaffold** discipline, and the
**mandatory end-state assertion** rule.

### Adding a test to the self-scoped integration project (`--project=integration`)

If the suite has a self-scoped Playwright project that drives a real {{VENDOR_B}} flow across several
services ({{INTEGRATION_PIPELINE}}: backend -> message queue -> integration gateway -> {{VENDOR_B}} mock,
optionally with an object-storage leg), it has its own stack, seeds, and reset. Do NOT scaffold a spec
for it from the generic flow - read first:
- `{{E2E_ROOT}}tests/integration/` folder doc - spec structure, per-variant record-id table, reuse map.
- The repo README section on running the integration project - the canonical boot/run recipe.
- Skills `starting-the-e2e-backend` and `running-e2e-tests` (their integration-project sections) -
  stack boot + Vite/run.

Non-negotiables for an integration-project spec: reuse the `playwright/support/` leaves (backend, orders,
imports, `VendorBMock`, gateway helpers) via the one shared `fixtures.ts` - never a
`tests/integration/support/` fork; import dep-clean leaves only (rule 13); assert every outbound
{{VENDOR_B}} call at `vendorBMock.expect*` with status (rule 17) and the outcome through the real UI
(rule 14); run specs that share mutable mock state (e.g. order specs vs import specs) in SEPARATE
invocations; follow the project's documented host/address and `{{FRONTEND_ENV_VARS}}` requirements.

## HARD RULE: a superficial test is worse than no test

**The moment you notice an assertion is superficial, deepening it is part of THIS task.** Not a flagged
gap, not a "known residue", not a follow-up ticket, not a cost to put to the coordinator. Superficial
tests are the worst kind of tests to have.

A test is superficial - and must be deepened before you report it done - if ANY of these is true:

1. **No demonstrated pre-fix failure.** It was never RUN against code where the guarded bug exists, so
   nothing proves it guards anything. Reasoning that it *would* fail is not a measurement, however
   sound. **If an earlier assertion in the same test aborts on pre-fix code so your assertion is never
   reached, the test is not proven - SPLIT it so each rule has its own independent pre-fix proof.**
   Consolidation (below) is only legitimate when every consolidated rule already has one.
2. It asserts a value the test itself seeded (assert the rule, not the row).
3. It passes **vacuously** when the row/element is absent, instead of failing.
4. It asserts presence only (`toBeVisible()`, "the button exists") rather than the outcome of the flow.
5. An acceptance criterion's complement is untested.

"Proving it costs a backend re-provision" is a cost to pay, not a reason to ship it. If you genuinely
cannot deepen it in this task, that is a BLOCKER to raise - not a line in "what I did not determine".

---

## Step 1: Understand what needs to be tested

Ask the user:

1. **What feature / flow** should the test cover? Describe the scenario in plain English.
2. **Which domain area** - which folder under `{{E2E_ROOT}}tests/` should this live in? (look at the
   existing folders to suggest options)
3. **Smoke or regression** - should any of the tests be tagged `@smoke`?

That's enough to start. Do not ask for seeds, types, or fixtures upfront - investigate those yourself.

---

## Step 2: Investigate before scaffolding

Before writing any code, use your tools to gather what you need:

### Seeds
- Look at `{{SEED_DIR}}` in {{BACKEND_REPO}} to find seeds relevant to the scenario.
- If an existing seed covers the test's preconditions, use it.
- If no suitable seed exists, a backend seed must be created/extended - **route that through
  `editing-backend-seeds-and-mocks`** (it owns both the authoring rules and
  the mandatory coordinator declare->ack for editing files outside your worktree). Do **not** silently
  write to the backend repo.
- Do not assume a seed exists - always verify.
- **Verify the seed JSON shape includes all data the test needs.** Plain ActiveRecord objects in
  `{ }.to_json` only serialize columns - associations (e.g. `product`, `integration_configuration`) are
  **not** included unless explicitly added via the splat pattern
  (`{ **record.attributes, product: record.product }`). If a helper derives expected state from seed
  data, the seed must include those fields.

### Fixtures & Page Objects
- Read `playwright/support/fixtures.ts` to see available fixtures and their types.
- Look at existing page objects in `playwright/pageObjects/` to understand what's available for the
  relevant domain.
- If the test starts on a page that doesn't have a fixture yet, flag it: _"This would need a new
  fixture for `<PageObject>` - want me to add it to `fixtures.ts`?"_

### Domain model & expected states
When the test turns on how the feature is *supposed* to behave - item phases, unarchive rules, bundle
instance statuses, trigger types, permission edges, and what each state *means* - get the intended
behavior from {{INTENT_SOURCE}} via **{{INTENT_SKILL}}** (it carries the authority rule: code = how it
does behave, intent source = how it should behave; on a gap, **flag it**, don't silently follow either).

Use that to write **concrete expected values** in assertions instead of guessing (e.g. assert
`'pending'` not `'draft'`; assert `'paused'` not `'stopped'` for a published + past-start-date
unarchived item). If the intent source and the code disagree, surface the gap rather than encoding a
guess.

### Existing test patterns
- Read 1-2 existing test files in the same domain folder to understand the conventions in use there.

---

## Step 3: Scaffold the file

Follow the **"Standard test structure"** in the `{{E2E_ROOT}}` root doc and the **"E2E Tests
(Playwright)"** editor rules for imports, `describe`/`beforeEach` shape, `@smoke` tagging, `server.*`
transition helpers, and the "No locators in tests" rule. The points below are the ones that are easy to
get wrong or that this skill specifically enforces:

1. **Import `test` from fixtures**, not from `@playwright/test`:
   ```typescript
   import { test } from '~/playwright/support/fixtures';
   ```

2. **Module-level seed variable** - only if the seed returns data the test needs. Use the actual type
   from `~/cypress/common/types/seedTypes` (or `~/cypress/common/types/memberTypes` etc.). If the test
   doesn't need a module-level variable, don't add one. **Never declare seed types inline in the test
   file** - add them to `cypress/common/types/seedTypes.ts` and import from there. Name the type after
   the seed it mirrors (`ListOverviewSeed` <- `seeds/<area>/list_overview`);
   **do not add a comment naming the seed file** (see rule 10).

3. **Seed types must match the actual JSON shape.** If the seed embeds associations via the splat
   pattern (e.g. `integration_configuration` at the item level instead of nested under `product`), the
   TypeScript type and any helper functions must read from the **same path**. When you change the seed
   structure, update the type in `seedTypes.ts` and any helpers that derive expected state from seed
   data.

4. **Keep fixtures minimal.** Ideally only the starting page fixture in `beforeEach`. Tests should
   navigate through flows by creating new page object instances inline - not by accumulating fixtures.
   Only add a fixture if the page object is genuinely reused across multiple tests.

5. **`beforeEach` is flexible** - include only what the test needs: seeding is optional (only if the
   scenario requires preconditions); `changeTenantTo` only if the seed includes a tenant;
   `page.goto` + `validateVisible` only for the starting point; `userFlows` to log in as a specific
   user when required.

6. **Assert every effect of the action - banners/dialogs are SECONDARY.** One user action (a single
   button click) usually has MULTIPLE effects. A test must verify ALL of them, and above all the
   **primary, state-changing effect(s)** - never stop at a confirmation banner, toast, flash, or dialog.
   Those are *secondary* effects (the app saying "I accepted/scheduled it"); they prove the request was
   accepted, not that the thing actually happened. Asserting only the banner is the "assert the flash,
   not the effect" anti-pattern and is treated as an incomplete test.
   - **Classify the effects first.** For the action under test, enumerate what it changes: (a) persisted
     state / the UI reflecting it (item phase, list membership, status badge, count); (b) downstream
     integration calls (a request sent to {{VENDOR_A}}); (c) secondary UX (banner, toast, dialog).
     Assert (a) and (b); (c) is a nice supporting check but NEVER sufficient alone.
   - **Primary UI/state effect is mandatory.** Every test must close with an explicit assertion of the
     final state - `waitForItemPhase(item, 'archived')`, `verifyListedCount(n)`, the specific item
     leaving/entering a list, a status flip - not just that an action was triggered. A page object
     method like `archiveItem()` verifying a toast internally is NOT sufficient.
   - **Downstream integration effect - assert at {{VENDOR_A}} via the {{MOCK_TOOL}} transaction log.**
     When an action is *supposed* to send a request to the distribution API (publish, republish,
     stop/pause, archive, activate an item/destination), assert the vendor actually received it - for
     the SPECIFIC item (`itemUuid`) and its destinations - using the reusable vendor-A helper
     (`playwright/support/vendorAMock.ts`): `purgeLogs()` before the action, then
     `verifyVendorReceived{Active,Pause,Archive}Update({ itemUuid, destinationTypes })` or
     `waitForLogs(predicate)` after. Reference specs: `stopDestination.spec.ts`,
     `earlyTerminationDestinationStop.spec.ts`. If the action hits a vendor path/verb the helper doesn't
     model yet, **add a new verify method** (and, if needed, a new mock route) rather than dropping the
     assertion. It is fine to ALSO assert the "...job scheduled" banner - as a secondary check on top of
     the vendor-level and state-level asserts, not instead of them.
   - A republish test that asserts only the "...job has been scheduled" banner - never that any item was
     actually republished or that the vendor received the request - is useless.

7. **`test.slow()`** - add this inside tests that involve a full item lifecycle (creation ->
   transitions -> finished).

8. **No locators in tests.** Tests must never contain `page.locator(...)`, `getByTestId`, `getByRole`,
   or any other selector/locator. All selectors live in page objects; tests only call page object
   methods.
   - **Page- or context-scoped elements:** use private getters, e.g.
     `private get submitButton(): Locator { return this.page.getByTestId('submit'); }`.
   - **Table rows / dynamic `rowId`:** getters cannot take parameters - use **private methods** that
     return a `Locator`, e.g.
     `private integrationStatusDot(rowId: string): Locator { return this.integrationStatusCell(rowId).locator('span.rounded-full'); }`.
     Resolve `rowId` in the public method (`getItemRowId` etc.), then delegate to these helpers.
   - See `ListTables` (`integrationStatusDot`, `sourceCell`, `sourceBadge`, `tooltipPanel`) for a
     reference pattern.

9. **One seed, multiple variants / URLs** - follow the editor test rules (nested `describe`, top-level
   seed `beforeEach`, per-variant `goto`). Reference:
   `playwright/tests/frozenDomain/groups/groupDetailsIntegrationStatus.spec.ts`.

10. **Do NOT add comments. None.** A flat ban with no carve-outs - not "explain the WHY", not a
    user-behaviour note with a ticket link, not provenance, not a seed-file name. It covers specs, page
    objects, `seedTypes.ts`, fixtures, helpers - every repo file.
    - If a line needs explaining, **rename the symbol or restructure the code**. The `test('...')` title
      and the `expect`/page-object method names are where meaning lives.
    - If it still needs explaining, the prose goes in the **PR description**, the **{{TRACKER}} ticket**,
      or the **folder doc** (documentation, not code) - never inline. Inline provenance (tickets,
      `app/...:line` citations, commit-SHA pins, gap analysis) rots and buries the code.
    - Do not relocate the prose into `test.step` labels or padded variable names to keep it alive.
    - **Delete dead code - never comment it.**
    - An existing comment you did not write is left alone unless your change deletes the code it sits on.
    - **Review your own diff for added comment lines before pushing and delete them.** When resolving a
      merge conflict whose two sides are differing comment prose, **delete the comment** rather than
      pick a wording.

11. **Minimize workarounds - drive the REAL user flow.** A spec must exercise the feature the way a
    user does: click the real button, navigate the real screens, then assert the real outcome. **Do
    NOT** assert an end-state via a synthetic backend command / seed transition that bypasses the very
    UI action the test exists to prove. Before adding ANY workaround (a custom app command, a direct
    model mutation, a bypass flag, a shortcut seed), check for an existing sanctioned mechanism and use
    it:
    - **Async jobs:** click the real action, then drain via the canonical scheduler-drain seed
      (`{{DRAIN_COMMAND}}`), then poll the UI for the end-state - exactly how the item specs do it
      (`stopDestination`, `earlyTerminationDestinationStop`, `transitionBundleItems`, `createItem`). Do
      **not** trust the in-process async adapter to drain on its own, and do **not** write a per-test
      shim. If the drain needs a new job, **fold it into the seed behind `{{DRAIN_COMMAND}}`** (it's a
      scheduler) - not a separate seed.
    - If an async end-state genuinely has no UI surface that can be observed deterministically, do NOT
      fake the UI result in Playwright and do NOT write a backend unit test yourself (QA writes e2e
      only). Hand it to developers as a **coverage note** in the PR/ticket.
    - A truly unavoidable workaround must be minimized and its WHY named in the PR/ticket, and flagged
      to the requester.
    Rule of thumb: if a reader can't trace your assertion back to a real user action, you've built a
    workaround - fix it. A "repair" shim seed that asserts repaired badges without ever clicking the
    real repair button is a MAJOR workaround. Always use the application from a user's point of
    view.

12. **Consolidate tests that share seed + setup - don't split what one test can assert.** Seeding +
    sign-in + navigation is the expensive, flaky part of an e2e test; assertions against the
    already-loaded page state are nearly free. So:
    - **Do NOT create a standalone test whose only job is "the page/list/dashboard renders" or "the
      seeded rows are listed."** Assert that INLINE, at the top of a functional test that loads the page
      anyway - you're loading the list either way, so check its content before doing the test's real
      target. Extract the list assertion into a **reusable page-object helper** that takes the expected
      collection (pattern: `validateMembersList(members)` in `membersList.spec.ts`; add e.g.
      `validateItemsList(items)`), and reuse it wherever a full list is loaded.
    - **Do NOT split assertions across multiple tests that share the identical seed + `beforeEach` +
      page load.** If two tests seed the same data, sign in the same way, and `open()` the same page,
      only to assert on different elements of that same rendered state, MERGE them into ONE test that
      asserts everything. Two tests paying the same setup cost twice = wasted run time + double the
      flakiness surface, for zero coverage gain.
    - **When to keep them separate (the real bar):** genuinely different setup/seed (empty-state vs
      populated), a different user/role, a mutation that changes state for subsequent assertions, or
      concerns independent enough that a merged failure would be ambiguous to diagnose. The test is
      "same seed + same setup + same loaded state -> one test"; only a *different* precondition
      justifies a *separate* test.
    Rule of thumb: before adding a test, ask "does an existing test already load this exact state?" If
    yes, add your assertion there instead of paying setup again. Checking list content whenever you load
    it costs zero time, reduces flakiness, and reduces run time.

13. **Reuse the existing page object / fixture / method - search before you write one, never fork a
    parallel copy or a near-duplicate.** Before you write ANY page object, helper, method, login flow,
    or selector, search for one that already exists and use it. Almost every domain already has a
    canonical PO under `playwright/pageObjects/pages/` registered as a fixture in
    `playwright/support/fixtures.ts` (e.g. `recordPage: RecordPage`,
    `orderManagementPage: OrderManagement`), and shared auth/tenant helpers live in
    `playwright/support/userFlows.ts` (`loginWith*`, `changeTenantTo`). Pull the fixture in by
    destructuring; do not instantiate a second copy.
    - **A hard blocker to reuse is a bug in the shared code, not a licence to fork.** A known one is a
      `~/` barrel ESM error ("exports is not defined in ES module scope") that breaks a PO's import chain
      in some standalone Playwright projects. The fix is to make the shared file importable - import the
      dep-clean leaf, or split the barrel - **not** to hand-roll a `MemberRecordPage` next to the real
      `RecordPage`, a second `OrderManagement`, or a `signInMember` next to `userFlows`. Every forked
      copy is a duplicated selector that silently rots when the real page changes.
    - If the shared PO is genuinely missing a getter/method the test needs, ADD it to that PO - don't
      spin up a parallel class beside it.
    - **Before you write that new method, grep the PO AND its base / shared layer (the
      `ListTables`-style component, `userFlows`, the base fixtures) for one that already achieves the
      goal - or a superset of it.** If an existing method does the job, use it. If one does *almost* the
      job, GENERALIZE it (add a name-only overload, a `poll?: false` / `wait?: boolean` flag, an
      optional arg) rather than adding a second near-duplicate beside it. Two methods whose bodies are
      90% the same are a bug in waiting - when the page changes, one gets updated and the other rots.
    - **Red flag - the synthetic-argument tell:** if you find yourself fabricating a partial/fake object
      (e.g. `{ name, itemSourceType: 'StandaloneRecord', record: null }`) purely to force an existing
      helper's internals down the code path you want, you are reverse-engineering it, not reusing it.
      That means the right move is to generalize the real helper (give it a clean by-name entry point),
      not to wrap a hack in a new method. E.g. a by-name phase check belongs in ONE name-locating phase
      helper with a poll-vs-assert flag, not a third parallel method beside `waitForItemPhase`.
    Rule of thumb: if you're writing a selector or a login/session flow that the suite plainly already
    has, stop - you're forking.

14. **Never read or assert product state via the backend console / `eval` / a runner script.** The
    whole suite reaches the backend ONLY through named committed seeds (`server.seed('seeds/...')`) and
    their returned JSON. Running raw backend code (the seed endpoint's `eval` app command,
    `rails runner`, a direct `Model.find_by(...)`) to check what happened, or to fabricate a
    token/record, is banned.
    - **Assert what the user sees through the UI - it is the ONLY oracle for anything the page
      renders.** If a value shows up on a page (a status badge, name, date, amount, list membership,
      mapped field), assert it by reading the RENDERED UI via a page object - NEVER by reading the app's
      own GraphQL/REST response for that same value. The app's own API response is NOT a UI stand-in: a
      green payload with a broken render is a false pass, and the user does not care about the payload.
    - **The only legitimate non-UI assertions:**
      (a) **outbound third-party contract** - a request the app/gateway *sends to an external service*,
        asserted at that service's **{{MOCK_TOOL}} admin transaction log** for the corresponding mock
        server (the `vendorBMock` / `vendorAMock.ts` `expect*`/`verify*` helpers - this is rule 6's
        downstream integration effect, e.g. the gateway called {{VENDOR_B}}
        order-status callback with a 2xx). The user can't see an outbound call, so there is no UI
        oracle for it.
      (b) the app's **own** API/GraphQL used ONLY for **navigation/setup** - resolving a uuid or minting
        a page token to *open* the page the assertion then drives (e.g. `tenantItemBundleInstances` +
        `generatePreviewToken`). **Never duplicate a UI-renderable assertion with an
        app-response read**, and never use the app's own response as the end-state oracle for something
        the page shows. A value with **genuinely no UI surface anywhere** is not asserted from the app's
        response either: hand it to developers as a coverage note (QA writes e2e only).
    - Never read product state via the ORM. If the app shows an item as activated, assert the activated
      badge/section (`validateActivatedSectionElements`), not `Instance.find_by(...).activated?`. This is
      the same principle as rule 11 (drive the real user flow), applied to the assertion side. A helper
      like `readImportedRecord` that runs the app's own record GraphQL query and asserts
      type/state/name/amount/date off the payload - while the record page RENDERS all of it - is
      rejected: assert the rendered record page + its connected pages; reserve API checks for the
      outbound vendor mock contract.
    - **Setup data comes from the seed, and the seed RETURNS what the test needs** (uuids, tokens,
      tenant) in its JSON - the way a page-access seed returns its `.token`.
      If you need a value that the seed doesn't return yet, add it to the seed's returned hash (route
      the seed edit through `editing-backend-seeds-and-mocks`), don't mint it
      via `eval`.

    | Rationalization | Reality |
    |-----------------|---------|
    | "It's just setup, not an assertion" | Minting a token/record via `eval` still bakes a backend-internals dependency into the test. Return it from the seed. |
    | "The UI runs this same GraphQL query, so reading its response is equivalent" | The user sees the RENDERED page, not the query result. Rendering can break while the query passes - a green payload read is a false pass. Assert the page. |
    | "The UI doesn't expose this state" | Verify that first - most fields DO render somewhere on the page or a connected one. If a value genuinely has no UI surface anywhere, hand it to developers as a coverage note; if it's an outbound call to a third party, assert the {{MOCK_TOOL}} transaction log - never the app's own response as a page stand-in. |
    | "It's faster/more reliable than the UI" | The point of an e2e test is to prove the user-visible behavior. A green DB/payload read with a broken UI is a false pass. |

    Red flag: a spec file contains `eval`, `rails runner`, `Model.`, an app-command that returns raw
    model state, OR a helper that queries the app's own GraphQL/REST and asserts field VALUES off the
    response for something the page renders. All mean: replace with a rendered-UI assertion, an
    outbound-contract (mock transaction-log) assertion, or a seed-returned value. Asserting in the
    backend console is a no-go, in general.

15. **Keep the spec body declarative - it reads as a short list of intention-revealing page-object
    calls.** A reader (QA or dev) should follow the test top-to-bottom as a plain-English scenario.
    After the `beforeEach` seed + `changeTenantTo` + `goto`, each `test` body is a handful of named
    PO method calls - `recordPage.validateVisible()`, `orderManagementPage.clickManageInApp()`,
    `orderManagementPage.validateActivatedSectionElements(...)`. That is what the body IS.
    - Everything mechanical lives BEHIND a fixture / PO / seed, never inline in the spec: HTTP/transport
      firing, mock transaction-log filtering (put it behind a named mock helper like
      `expectDestinationsResolved(...)` - cf. the reusable `playwright/support/vendorAMock.ts` in rule
      6), `localStorage` session pokes (use `userFlows`), token minting (seed), DB/status polling (a
      PO/`server` wait helper).
    - A spec that interleaves API calls, log parsing, session injection and one final UI assert across
      many narrated `test.step`s is too low-altitude - lift each concern into the layer that owns it.
    Rule of thumb: if a `test` body contains `page.evaluate`, `localStorage`, a URL-string match on a
    request log, or raw HTTP, it belongs in a helper. Compare your spec against
    `main/orderManagement/orderManagementActivation.spec.ts` and `main/record/recordDetails.spec.ts` -
    if it doesn't read like those, flatten it. A spec must be readable by QA and devs.

16. **Cross-cutting cleanup belongs in the global before-each, not per-test - and don't write
    reachability pre-checks.** State-reset that every test needs runs once in the global `forEachTest`
    auto-fixture (`playwright/support/fixtures.ts`), which already does `server.seed('clean')`. If a
    test needs any OTHER shared reset - purging the mock transaction log, clearing an external stub -
    add it to that same global before-each so every spec starts clean, rather than repeating the purge
    inline in one spec. Per-spec cleanup that every spec would want is a smell.
    - **No reachability / "is the stack up" pre-check test or step.** If the backend/mock/Vite stack
      isn't reachable, the real test fails on its own at its first interaction - a separate
      `stackReachable` spec or a `beforeAll` "ping the stack" guard just adds a slower, redundant
      failure path. Delete it. (Same family as "no standalone page-loads test" in rule 12.)

17. **Prove EVERY third-party touch at that vendor's mock transaction log - mandatory, exhaustive, and
    at the moment it happens.** Whenever the flow under test is *supposed* to reach an external service
    ({{VENDOR_B}}, {{VENDOR_A}} - any third party the app or the integration gateway calls out to), the
    test MUST, right after the action that triggers the call, assert that service's {{MOCK_TOOL}}
    transaction log contains **ALL** the corresponding requests. This is not optional and not a
    nice-to-have: it is the ONLY proof that **we actually went to the third party and got the correct
    data back**. A green UI on its own does not prove it - the same UI can be satisfied by data that was
    seeded or injected in-process and never crossed the vendor boundary (the hole rule 6 addresses for
    {{VENDOR_A}}; this rule generalizes it to every third party).
    - **One assertion per outbound call the step makes - exhaustively.** If ordering for an active
      record resolves the record AND its destinations AND posts a status callback, assert all three
      (`expectRecordResolved`, `expectDestinationsResolved`, `expectOrderStatusPosted`) - not just the
      first. If a step legitimately makes fewer calls (a record in a pre-active state has no live
      destinations, so no destinations fetch), assert exactly the set it *should* make and say why in
      the test name, so a missing call trips instead of passing silently.
    - **"Correct data" = the real response + the rendered outcome, together.** The log assertion proves
      the call was made and got a real success (assert method + path + a real status - `200`/`204`,
      never a 404 fall-through), count-scoped so stale traffic can't green it (`purgeLogs()` per test -
      put the purge in the global before-each per rule 16). That the *correct* data came back is then
      proven by the rendered-UI assertion (rule 14) downstream of the call. The pair - outbound-log hit +
      the value it produced appearing on the page - is what "we reached the vendor and got the right
      data" means.
    - **Use the reusable vendor helper; add to it, never drop the assert.** These live behind a
      `MockClient` subclass per vendor (`playwright/support/vendorBMock.ts` -> `VendorBMock`;
      `playwright/support/vendorAMock.ts` -> `VendorAMock`), with `waitForLogs`/`purgeLogs` in the base.
      If the flow hits a vendor path/verb no helper models yet, **add a named `expect*`/`verify*`
      method** (and a mock route if needed) - do not skip the assertion because a helper is missing.
    - **If a spec's flow SHOULD touch a third party but has NO such log assertion, that is a coverage
      hole - HIGHLIGHT it to the operator, do not paper over it.** The classic case: a spec whose seed
      injects the end-state in-process (e.g. an "import" seed that calls the backend's inbound receive
      service directly) so the vendor is never actually called - the UI renders, but nothing proves we
      reached the vendor. Flag it explicitly (like rule 6's vendor gap) rather than treating the UI
      render as a substitute for the missing outbound proof.
    Reference specs: the `@smoke` order specs in `{{E2E_ROOT}}tests/integration/`.

18. **Keep the folder doc current - update it in the same change.** Every folder under `{{E2E_ROOT}}`
    carries a doc (the folder's `CLAUDE.md`) documenting what the folder holds and how its files are
    used. When you add, move, rename, or delete a file in an e2e folder (a new spec, page object,
    fixture, factory, model, or support helper), update that folder's doc in the SAME change so the docs
    never drift from the tree - the file inventory and any "how to use" notes must still match. If you
    create a brand-new folder, give it a doc too (same structure as its siblings, ending with the
    standard `> **Maintenance:** ...` footer). This is the prose keep-current mechanism the per-folder
    docs rely on; there is no CI check enforcing it, so it is on you.
    **Verify run-recipe docs on the operator's ACTUAL environment, not just yours.** When a folder doc
    or README change documents *how to run something locally*, confirm it on every environment the
    team really uses (e.g. containerized vs native services, each developer OS), not the author's
    convenience path. A "how to run" doc that's wrong on the operator's host is a correctness bug, not a
    nit.

19. **Assert the RULE, not the ROW - never let the test supply its own expected value for a field the
    SYSTEM derives.** A round-trip assertion (`fill(x)` ... `expect(field).toHaveValue(x)`, or seed `x`
    then assert `x`) proves only that storage and rendering agree. **It is structurally blind to a wrong
    value the system computed for itself** - exactly the class of bug where a field is populated by an
    integration, a URL template, a precedence rule, or a fallback branch.
    - Ask before writing the assert: *"where does the expected value come from?"* If the answer is "the
      test typed it" or "the seed set it", and the field is one the app derives, the test cannot fail
      for the reason that matters.
    - Derive the expectation from the **documented rule** instead, and assert the invariant - not
      *"the generated link equals what I just typed"* but *"the generated link matches the tenant's
      configured URL template, and is not an external or preview URL."*
    - Round-trip IS the right assert when the user's own input is the feature (editing a custom URL,
      renaming, free-text fields). The rule applies to derived/computed/integration-populated values.
    - **And check the fixture can even produce the failure:** if the seed or vendor mock only ever emits
      one canonical variant, the risky branch is unreachable and the spec is green by construction. Add
      the variant (see `editing-backend-seeds-and-mocks`) rather than
      declaring it untestable.
    *Why:* round-trip asserts on a derived field stay green throughout the very leak they sit on.

---

## Step 4: Review with the user

After presenting the scaffolded file:
- If a new backend seed is needed, confirm its path and what it returns, and confirm the declare->ack
  path via `editing-backend-seeds-and-mocks` was followed.
- Highlight if a new fixture needs to be added to `fixtures.ts`.
- Note any placeholder TODOs the user needs to fill in.
- **Third-party log coverage (rule 17):** if the flow touches an external service
  ({{VENDOR_A}}/{{VENDOR_B}}/...) at any step, confirm every such touch has a mock-transaction-log
  assertion. If a step that *should* reach a third party has none - most often a seed that injects the
  end-state in-process so the vendor is never called - **highlight that gap to the operator
  explicitly.**
- **No-UI end states (rules 11, 14):** list any end state that has genuinely no UI surface as a coverage
  note for developers in the PR/ticket.
- **Folder doc updated (rule 18):** if you added/moved/renamed/deleted any file (or created a new
  folder), confirm the affected folder's doc was updated in the same change.
- Ask if they want you to implement any of the flagged missing pieces.

## Related skills

- `running-e2e-tests` - run the spec you scaffolded.
- `starting-the-e2e-backend` - bring up your own per-{{SLUG}} backend for the run.
- `investigating-e2e-failures` - root-cause a red run.
- `qa-e2e-test-only-changes` - where QA edits may and may not land.
- `reviewing-e2e-coverage` / agent `senior-qa-e2e-reviewer` - audit the finished spec's depth.
