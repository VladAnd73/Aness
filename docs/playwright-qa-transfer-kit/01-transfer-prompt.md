# Transfer prompt: build a Playwright e2e QA structure on a new project

> Paste everything below the line into the receiving spore coordinator. It assumes nothing from the source host: no repo, skills or memory. Placeholders are `{{LIKE_THIS}}`; resolve each in Phase 0 before building. The kit README carries the placeholder glossary.

---

You are the QA coordinator for a **new** product, `{{PROJECT_NAME}}`. Your job is to stand up a Playwright end-to-end suite and its CI, documentation layer and agent tooling. The structure must match a reference suite that a team already runs in production. That reference cannot be shown to you. This prompt is your only source for it, together with the files of this kit that the phases below name. **Anything this prompt does not say does not exist; do not invent extra machinery.**

The prompt has three parts:
- **(A)** conventions and architecture to replicate **as-is**;
- **(B)** reference-specific bindings you must **re-derive** for this project;
- **(C)** an executable plan in phases, each with a done-check.

Work through (C) in order. Do not start a phase until the previous phase's done-check passes. Record evidence for each done-check: a command output, a path, a CI run URL.

## Ground rules for you and your workers

- **Every claim about the new project carries a source:** a `file:line`, a command output or a URL. Label inferences as hypotheses.
- **Never merge.** Only the operator merges. Pushing to feature branches and opening PRs is fine. PRs target `{{DEFAULT_INTEGRATION_BRANCH}}`.
- **Developer tickets are read-only for QA.** Report bugs in new tickets.
- **QA does not edit product source to make a test pass.** If the durable fix is in the app, document it for developers and keep the QA change test-side.
- **When you find a real bug while writing tests,** surface it: use `test.fixme` plus a ticket. Never make the test green by matching the buggy behaviour.
- **Ask the operator only for decisions that are genuinely theirs** (Phase 0 lists them). Otherwise take the conventional default and say so.

---

## (A) Replicate as-is: architecture and conventions

### A1. Folder layout

```
<e2e-root>/                      (the reference used `playwright/` at the frontend repo root)
|-- CLAUDE.md                    suite index + root conventions (see A10)
|-- tsconfig.json                path aliases for the suite
|-- reporters/                   custom reporters (only if you adopt timings; see Phase 3 "full")
|-- support/                     ALL infrastructure. The ONLY place helpers/fixtures live.
|   |-- fixtures.ts              the single shared `test` object (auto per-test reset + named fixtures)
|   |-- server.ts                seeding client: seed(name, options) -> JSON; async-drain helper
|   |-- backendApi.ts            dep-clean low-level request primitives (command POST, sign-in, API query)
|   |-- userFlows.ts             login shortcut, role users, active-tenant switch, clearSession
|   |-- mockClient.ts            dep-clean base client for the mock server's admin/log API
|   |-- <vendor>Mock.ts          one assertion class per third-party mock (verify*/expect*)
|   |-- projects.ts              Playwright project-name constants
|   |-- global-setup.ts          (optional) CI-only warm-up
|   `-- utils.ts                 suite-own dep-clean utilities (formatters, oracles)
|-- pageObjects/
|   |-- BasePage.ts              base class every page object extends
|   |-- pages/<Domain>/          one class per route/screen (plural file = list, singular = detail)
|   `-- components/              drawers/ modals/ sections/(tables) headers/ navigation
|-- data/
|   |-- factory/                 faker builders that produce FORM INPUT (never backend rows)
|   |-- models/                  TS types + preset constant arrays (no logic)
|   |-- fixtures/                static canned payloads for page.route() mocks + binary upload assets
|   `-- types/seeds.ts           TS shape of every seed's returned JSON (never declared inline in a spec)
|-- tests/
|   `-- <area>/<domain>/*.spec.ts  specs grouped by product domain
`-- prodSmoke/                   (optional, later) read-only production smoke with its OWN config
```

Rules:
- **Never fork a second `fixtures.ts`** and never add a `support/` folder under `tests/`.
- A new fixture, driver or helper goes on the one shared fixture object. Playwright fixtures are lazy, so an unused fixture costs nothing.
- **Seed result types live in one shared types file** and match the seed's real JSON shape.

### A2. Playwright config shape

- **Default project:** a single `chromium` project (Desktop Chrome, fixed locale, fixed viewport).
  - `testDir` is the whole `tests/` folder.
  - It is **fail-open**: it collects everything except folders owned by another self-scoped project, via `testIgnore`.
  - Never narrow it to named folders. A spec outside them would be collected by no project, with no warning and exit code 0.
  - Adding a second project means giving it its own `testDir` **and** adding that path to chromium's `testIgnore` in the same change.
- **Project names are exported constants** in `support/projects.ts`. Any fixture gated on a project compares against the constant, never a string literal.
- `fullyParallel: true`.
  - **`workers`:** see coupling B2. The reference uses `1`, because it truncates the DB before every test.
- `forbidOnly: !!CI`.
- `retries`: CI 2, local 0.
- `timeout`: CI 60 s / local 40 s.
- `expect.timeout`: CI 60 s / local 20 s.
- `maxFailures`: the reference uses 3 on CI. Start without it until the suite is stable, because early-stopped shards make run-to-run diffs show fake "not run" blocks.
- `use.baseURL`: `{{FRONTEND_URL}}`.
- `use.trace`: `retain-on-failure`.
- `reporter`: CI `blob`, local `html`.
- `webServer`: boots the frontend dev server, with `reuseExistingServer: !CI`. **The backend URL passed to the frontend must come from an env var with a default, never be hard-coded**, so that a non-default backend port is honoured.
- **No `storageState` login.** Auth happens per test in the auto fixture (A4).
- Load the suite env from a gitignored `<e2e-root>/.env`. **Commit a `.env.example`** listing every variable (the reference lacked one; do better).

### A3. Page Object Model rules

- Every PO `extends BasePage`, is a `default` export, and calls `super(page)`.
- **Selectors are private getters** (`private get saveButton(): Locator`). Use private methods only when a selector needs an argument.
  - Prefer `getByRole` / `getByTestId`.
- **Specs contain no locators at all:** no `page.locator`, `getByRole` or `getByTestId` in a spec. A spec reads as a plain-English list of PO method calls.
- **Navigation methods return the next PO** (`startCreate()` -> `CreateDrawer`).
- **Action methods follow one pattern:**
  1. register the network wait for the specific request **before** the click;
  2. click;
  3. await the response;
  4. assert the user-visible confirmation.
  The confirmation is a **secondary** check only.
- **Polling helpers** `waitFor<Thing>(entity, expected, { poll?: boolean })` reload or poll until the UI shows the expected state. Derive display strings with a suite util instead of hard-coding them.
- **Reuse before you write.** Before adding a `validate*` / `verify*` / `expect*` / `wait*` helper, grep for one covering the same locator, log predicate or seed concern. **Extend it with an optional parameter** instead of adding a sibling. Two helpers over one locator drift apart.
- POs registered as fixtures are destructured in specs. Others are created inline with `new X(page)`. Never create a second copy of one that is already a fixture.
- `BasePage` provides:
  - named network waits;
  - `reloadUntil(assertion, timeout)` (`expect(...).toPass` with reload) for pages that don't live-update;
  - multi-element waits;
  - `route.fulfill` intercept helpers that serve canned payloads from `data/fixtures/` (only for upstreams you deliberately don't exercise).

### A4. The shared fixture and per-test lifecycle

`support/fixtures.ts` exports `test = base.extend(...)` with one **auto** fixture, `forEachTest`, that runs before every test:
1. **Reset backend state** (the reference: `server.seed('clean')`, a truncation).
2. **Purge every configured mock's transaction log**, and reset stateful mocks. A mock whose base-URL env var is unset makes this a **no-op**, so the same fixture works on runs that don't boot that mock.
3. **Log in**: create a privileged user via a seed, sign in through the backend API, and plant the credential the app reads before the first navigation. The reference fulfils a fake `/__pw_setup` route to get the app origin, then writes `localStorage`.
4. `await use()`.
5. Teardown: only optional coverage collection. **Isolation is clean-before, not clean-after.**

Named fixtures: `server`, `userFlows`, one per mock-assertion class, and the frequently used POs.

**Every spec imports `test` from `support/fixtures`, never from `@playwright/test`.** Cross-cutting resets every test needs go in `forEachTest`, never repeated in specs. There are **no reachability pre-check tests or `beforeAll` pings**: a down stack fails at the first real interaction anyway.

### A5. Standard spec shape

```ts
import { test } from '<alias>/support/fixtures';
import type { SomeSeed } from '<alias>/data/types/seeds';

let seeded: SomeSeed;

test.describe('Domain | Feature | Action', () => {
  test.beforeEach(async ({ page, server, userFlows, listPage }) => {
    seeded = await server.seed('seeds/<domain>/<scenario>', { /* options object whenever the seed reads one */ });
    await userFlows.changeTenantTo(seeded.tenant);   // only if the seed returns one
    await page.goto(listPage.url);
    await listPage.validateVisible(false);
  });

  test('does X and the user sees Y @smoke', async ({ server, vendorMock, listPage }) => {
    await listPage.waitForPhase(seeded.entity, 'live');
    await listPage.pause(seeded.entity);          // real UI action
    await server.runScheduler();                  // deterministic async drain (see B4)
    await listPage.waitForPhase(seeded.entity, 'paused');                 // primary end state, via UI
    await vendorMock.verifyReceivedPause({ entityId: seeded.entity.id }); // outbound contract, via mock log
  });
});
```

The inline `//` notes above annotate this prompt only. Real specs carry no comments (A7).

- **`@smoke`** in the test title means it runs on every PR. Untagged means full regression only. Use `test.slow()` for long lifecycle tests.
- **Seed call gotcha:** pass an options object whenever the seed reads one. Omitting it can send no `options` key at all and crash the seed before its defaults apply.

### A6. Assertion rules (the normative core; replicate verbatim in spirit)

1. **Every acceptance criterion gets a behavioural assertion,** including the ACs of tickets this change links to. Never assert only that a control exists, is visible or is enabled.
2. **Every AC that names a population gets its complement tested.** "Applies to premium accounts" also claims "standard accounts are unaffected". If no seed can produce the complement, extend the seed.
3. **Assert the rule, not the row.** If the test supplied the value (fill, factory, seed) and the field is system-derived (integration, URL template, precedence, fallback), a round-trip assert is structurally blind. Derive the expectation from the documented rule.
   - Round-trip is correct only where user input *is* the feature (rename, custom URL, free text).
4. **One action, many effects: assert them all.** The primary persisted state change (as rendered) and the outbound third-party call are mandatory. A toast or banner is secondary and never sufficient alone.
5. **The rendered UI is the only oracle for anything the page shows.**
   - Never assert a UI-renderable value from the app's own API/GraphQL response.
   - Never read DB or console state.
   - Legitimate non-UI asserts are only (a) outbound third-party calls at the mock's transaction log, and (b) values with genuinely no UI surface.
   - The app's own API may be used only to navigate or set up (resolve an id, mint a page token).
6. **Prove every third-party touch at that vendor's mock transaction log.** Each verifier does three things:
   - matches the **specific entity** (id or name), not the path alone;
   - asserts the **response status**, so a 404 fall-through fails;
   - **polls until all expected calls arrive**, with a `minMatches` count so exact-count asserts can't race.

   Every positive verifier has a **negative counterpart**. If a vendor path isn't modelled, add a verifier; never drop the assertion.
7. **A test with no demonstrated pre-fix failure is superficial.**
   - Run every regression assertion against code carrying the bug and show it red before landing it. "It would fail because..." is reasoning, not a measurement.
   - An assertion that sits behind an earlier one which aborts on pre-fix code never ran, so split the test until each rule fails on its own.
   - Where this rule and rule 2 collide, this rule wins: a complement identical with and without the fix can't fail.
8. **Deepen a shallow assert in the change that touches it.** That covers presence-only asserts, values the test supplied, and asserts that pass vacuously when the row is absent. "Pre-existing" is no defence. **A known-superficial test is never shipped or deferred.**
9. **Read the fixture, not just the assertion.** A seed or mock that can only produce the happy variant makes the test incapable of catching the bug. A failure mode the fixtures can't produce is a **fixture gap to close**.
10. **Before calling screen behaviour a bug,** check that the seed built a state the product can actually reach.
11. **Drive the app like a real user, with as few workarounds as possible.**
    - Click the real action, then drain async work through the **one sanctioned drain mechanism**, then poll the UI.
    - Never replace the action under test with a backend shortcut.
    - If a drain needs a new job, fold it into the existing drain command rather than writing a per-test shim.
    - An unavoidable workaround is minimised and its reason goes in the PR.
12. **Derive formatted expectations in the browser** (dates, prices, phone numbers) using the page's own locale. Never hard-code a string that encodes the runner's locale.

### A7. Code hygiene rules

- **No comments in test code**: specs, POs and support files. A flat ban with no exceptions; prose goes to the PR, the ticket or the folder `CLAUDE.md`.
  - Never explain product source in tests (component names, props, render timing, test-id schemes).
  - Provenance (tickets, SHAs, root-cause analysis) goes in the PR or the ticket.
  - Conventions go in the folder `CLAUDE.md`.
  - Don't smuggle the prose into test names, step labels or variable names. Rename instead.
- **Consolidate tests that share seed and setup.** Seeding, sign-in and navigation are the expensive, flaky part.
  - Never write a standalone "page/list loads" test. Assert list content inline at the top of a functional test, via a reusable PO helper such as `validateXList(items)`.
  - Two tests with identical seed, `beforeEach` and page load are merged.
- **Test code never imports the app's runtime barrels** (index files with side effects or env config). One bad import breaks the whole import chain and Playwright reports "No tests found". Keep suite-own dep-clean util leaves. Import app files only if they are type-only or dependency-free leaves.
- **Settle any Playwright API question from `node_modules`** (the installed version's `.d.ts` and lib), not from memory.
- **Lint, format and type-check the e2e folder in CI.** The reference did not, so its no-locator and no-comment rules were enforced by review alone. You must add the backstop: ESLint `no-restricted-syntax`/`no-restricted-properties` for `page.locator|getBy*` in `tests/**`, plus `tsc -p <e2e-root>`.
- **Skipped tests** (`.skip`/`fixme`) are documented gaps; the reason goes in the PR or ticket. When a whole area is out of scope, `describe.skip` is the mechanism. Never touch the CI or config to hide it.

### A8. Seeding pattern (architecture to replicate; the mechanism is re-derived in B1)

- The suite reaches the backend **only** through **named, committed, versioned seed scripts** in the backend repo. Each builds one scenario and **returns the JSON the spec needs** (ids, tokens, tenant). If a spec needs a value, the seed must return it.
  - Arbitrary code execution and ad-hoc DB reads from tests are banned.
- **Seed kinds:**
  1. **Setup seeds** per scenario, in folders by domain.
  2. **Transition seeds**, in their own folder, for state advancement only: drain schedulers, force a phase, and a read-only status getter used for *waiting*, never as the assertion.
  3. **Generic commands:** `clean` / reset, factory call.
- **Seed authoring rules:**
  - enable feature flags before creating data;
  - use the backend's factory library with traits, never raw inserts;
  - navigate associations rather than creating orphans;
  - comment any validation bypass in the seed (backend repo, not test code);
  - take parameters from an options object, with required params fetched strictly;
  - return a **minimal** hash with clear keys;
  - prefer a focused new seed over conditional branching.
- **The seed client names the failing command** in its error. Seeds are the top source of e2e failures, and a URL-only error makes them all look alike.
- **Expect three seed-endpoint failure modes** and document them in `support/CLAUDE.md`:
  - "command not found": the endpoint is up and the name is wrong;
  - "not mounted": wrong env or backend down;
  - "script raised": a seed bug.

### A9. Mock-server pattern

- Third parties are replaced in e2e by HTTP mock servers. The reference uses **Mockoon**: JSON environments, an admin API with a transaction log, and log/state purge.
- **Mocks live in one canonical home** (the reference used a dedicated mocks repo). They are never vendored into the frontend, even temporarily.
- **Routes are built from the vendor's own API documentation,** with the source cited in the route's description. If our code diverges from the vendor contract, that is a bug in our code: flag it and do not bend the mock.
- **Responses are dynamic.** Echo request values (ids, names) rather than hard-coding them. Keep templated output valid JSON. Preserve the caller's correlation ids.
- **After any mock edit:** restart the mock, hit the route, and re-run the spec to prove it.

### A10. Per-folder CLAUDE.md discipline

- **Every folder under `<e2e-root>/` has a `CLAUDE.md`.** Shape:
  - `# <path> - <role>` plus a parent link;
  - `## What's here` (each file/subfolder: one line naming its key classes and methods);
  - `## How to use it`;
  - `## Conventions & gotchas`, **prescriptive** rules at the decision point, not narration;
  - `## Cross-links`;
  - a footer.
- **Footer**, one of two standard lines:
  - content folders: `> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.`
  - index folders: `> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.`
- **Any change that adds, moves, renames or deletes a file updates that folder's `CLAUDE.md` in the same change.** A new folder gets its own file.
- **State the rule, not the incident.** The story behind a rule goes in the PR or ticket.
- **The root e2e `CLAUDE.md` carries:**
  - the folder map;
  - the projects;
  - the import-alias table and barrel trap;
  - the fixture wiring (which fixtures exist);
  - the standard spec shape;
  - how seeds work;
  - the numbered suite-wide conventions (A6 + A7);
  - the CI behaviour summary.

---

## (B) Re-derive per project: bindings and couplings

Each binding below names the **role** it played in the reference, the **precondition** the new project must offer, the **fallback** if it can't, and the **question** to resolve in Phase 0.

| # | Role (reference binding, for orientation only) | Precondition | Fallback | Placeholder / question |
|---|---|---|---|---|
| B1 | **Seed endpoint:** a test-only backend endpoint that runs a named, committed scenario script in the backend process and returns JSON (reference: Rails `cypress-on-rails` `POST /__cypress__/command`, scripts in `spec/cypress/app_commands/seeds/**`) | Non-production-only; access to domain factories; returns JSON | (a) a seed library in the test repo over a public/admin API; (b) per-scenario SQL fixtures; (c) DB snapshot restore. Keep "named + committed + returns what the spec needs". | `{{SEED_MECHANISM}}`: does the backend offer, or will it accept, a test-only command endpoint? Which language and factory library? |
| B2 | **Per-test reset + isolation:** full DB truncation before every test, which forces `workers: 1` and makes CI parallelism come from shards with their own DB | A reset that is fast and total, **or** cheap per-test tenant creation | **Per-test tenant isolation** (a fresh org per test, no truncation), which allows `workers > 1` | `{{ISOLATION_STRATEGY}}`: truncate + serial, or tenant-per-test + parallel? This decides the CI shard math. |
| B3 | **Auth shortcut:** API sign-in returns a bearer header; the SPA reads it from `localStorage` (reference: one key for the token, one for the active tenant) | A programmatic sign-in producing what the app reads | Cookie or session apps: `context.addCookies` / per-role `storageState` from an API login. SSO-only: a test-env bypass, or one UI login per role in a `setup` project. | `{{AUTH_MECHANISM}}`, `{{TOKEN_STORAGE_KEY}}`, `{{TENANT_SWITCH_MECHANISM}}` |
| B4 | **Async drain:** a transition seed that runs scheduler jobs synchronously, plus a test-env in-process job adapter enabled by an env flag | A deterministic way to run or drain background work in test mode | Long UI polls against real workers. A job that can't be drained deterministically is recorded as a coverage gap for developers, not faked. | `{{ASYNC_JOB_SYSTEM}}`: how are jobs run in test mode, and can a seed drain them? |
| B5 | **Third-party mocks** (reference: one Mockoon environment per vendor, each on its own port: a distribution API that receives the product's outbound updates, and an upstream system-of-record vendor used only by the integration project) | **The backend's outbound base URLs must be env-overridable** in test mode, and the mock must have a queryable request log plus reset | WireMock / MockServer / a recording proxy. If URLs are hard-coded, raise a testability ticket. | `{{THIRD_PARTIES}}` (list), `{{MOCK_TOOL}}`, `{{MOCKS_HOME}}` (repo/path), per vendor `{{VENDOR_API_DOCS_URL}}` |
| B6 | **CI backend:** a prebuilt backend Docker image in test mode run as a GitHub Actions service, with Postgres + Redis services (reference: a registry image on a **floating** `development-test` tag; that caused un-attributable reds) | A runnable backend artifact for CI, **pinned by digest or version** | Build the backend in-job, or docker-compose the backend repo at a pinned ref | `{{CI_BACKEND_ARTIFACT}}`, `{{CI_PLATFORM}}`, `{{CONTAINER_REGISTRY}}` |
| B7 | **Frontend runtime env:** the dev server needs specific env (map-tiles tokens, URLs of sibling apps). Missing values silently blank pages and look like regressions. | A canonical env list shared by CI and local | none | `{{FRONTEND_ENV_VARS}}`, `{{FRONTEND_URL}}`, `{{BACKEND_URL}}` |
| B8 | **Privileged admin UI** on the backend origin, with a passwordless test-only sign-in route | Only if the product has one | Treat it as a second origin with its own login helper | `{{HAS_SERVER_ADMIN_UI}}` |
| B9 | **Multi-service integration project:** a self-scoped Playwright project for an async integration pipeline, with its own label-gated workflow (reference: integration gateway + message queue + S3 emulator + vendor mock) | Only if there is such a pipeline | Defer until Phase 4 "full" | `{{INTEGRATION_PIPELINE}}` |
| B10 | **Post-deploy production smoke:** its own config and folder, read-only, no backend/fixtures, canary data | Production canary data + a deploy hook | none | `{{PROD_SMOKE_WANTED}}`, `{{CANARY_IDS}}` |
| B11 | **Product-intent oracle** used by coverage reviews and bug hunts (reference: a product-knowledge wiki). Code shows how the product *does* behave, the KB how it *should*; on conflict, flag the gap. | Some written intent | Ticket ACs + designs; flag missing intent | `{{INTENT_SOURCE}}` |
| B12 | **Tracker** for bug filing and AC reading (reference: one issue tracker, with some work dual-tracked in a second one) | none | none | `{{TRACKER}}`, `{{TRACKER_PROJECT_KEY}}` |
| B13 | **Local runtime:** a pinned dev shell with a chromium matching the Playwright version (reference: a NixOS flake devshell), a per-consumer isolated backend (own DB/cache/port derived from a unique key), and one dev-server-plus-run per port | Host-specific | Install browsers with `npx playwright install chromium` | `{{HOST_OS}}`, `{{LOCAL_BACKEND_BOOT}}` |
| B14 | **Report hosting:** merged HTML on GitHub Pages under a unique per-run folder, pruned on a schedule | Pages enabled | Artifact-only | `{{REPORT_HOSTING}}` |
| B15 | **PR label vocabulary:** `approved` (auto-added after 1 review; gates smoke), `skip tests`, `{{FULL_REGRESSION_LABEL}}`, `{{RELEASE_TESTS_LABEL}}`, `{{INTEGRATION_LABEL}}` | Repo labels + a token able to label | Run smoke on every push | `{{LABEL_POLICY}}` |

**Couplings you must not miss:**
- **B1 -> A6 rule 5.** The ban on DB and console reads is livable only because seeds give a sanctioned channel for *waiting*: a status-getter seed. Without B1 you still need a sanctioned wait channel, or "UI is the only oracle" degenerates into unbounded UI polling.
- **B2 -> `workers` -> CI shard count.** Decide B2 before writing CI.
- **B5 -> A6 rule 6.** "Every third-party touch asserted at the mock log" requires a **backend** testability property (overridable outbound URLs). If it's missing, that is a product ticket, not a test workaround.

---

## (C) Executable plan

### Phase 0: Discovery (questions only; build nothing)

Inspect the new project's repos and resolve **every** `{{placeholder}}` in A and B. Ask the operator only for:
- B2 isolation strategy, if both are feasible;
- B6 backend artifact strategy;
- B10 prod smoke, yes or no;
- B15 label/approval policy;
- whether the e2e suite lives in the frontend repo or its own repo (`{{E2E_REPO_LOCATION}}`; the reference lived inside the frontend repo).

Also determine:
- `{{FRONTEND_STACK}}` (framework, dev server, port, how it reads the backend URL);
- `{{BACKEND_STACK}}`;
- `{{APP_API_STYLE}}` (REST/GraphQL; this decides `BasePage`'s network-wait helper);
- `{{UI_LIBRARY_PORTALS}}` (drawers or menus rendered in portals, which become flaky-interaction candidates);
- whether pages scroll the window or an inner container;
- whether a Suspense or loading fallback duplicates the app chrome (both are known false-pass sources);
- `{{ROLES}}` and `{{TENANCY_MODEL}}`;
- the first **3 user journeys** to cover (`{{FIRST_JOURNEYS}}`), at least one of which hits a third party.

**Done-check:** a `discovery.md` exists listing every placeholder with its resolved value and a source for each (file:line, URL or operator quote). Every precondition in B1-B5 is marked *met*, *fallback chosen* or *ticket filed*.

### Phase 1: Scaffold

1. Create the A1 tree, `tsconfig.json` with an alias for the suite root, and the Playwright config per A2. Pin `@playwright/test` to an exact version.
2. Create `.env.example` with every variable the suite reads.
3. Implement `support/` per A4 and A8:
   - `backendApi.ts`, dep-clean: `postCommand`, which names the command in errors; `unwrapSeedResult`; `signIn`; and the API query helper, which throws on API-level errors even on HTTP 200.
   - `server.ts`: `seed`, `runScheduler` (or the B4 equivalent).
   - `userFlows.ts`: `loginWith`, `loginWithCredentials`, `createUserWithRole`, `changeTenantTo`, `clearSession({cookies})`.
   - `mockClient.ts`: `purgeLogs`, `purgeState`, paginated `getLogs` with a runaway backstop that throws, `waitForLogs(predicate, {minMatches})`, and `dispose`. An unset base URL makes purges no-op and reads throw.
   - `fixtures.ts`: auto `forEachTest`: reset -> purge mocks -> login -> `use`.
   - `projects.ts`.
4. Implement `BasePage` with network waits, `reloadUntil` and multi-element waits. Add only what the app needs.
5. Add CI lint, format and type-check coverage for the e2e folder, including the "no locators in `tests/**`" lint rule (A7).

**Done-check:**
- `npx playwright test --list` succeeds and lists 0 tests without errors;
- `tsc -p <e2e-root>` and lint pass;
- a throwaway spec calling `server.seed('clean')` plus login and `page.goto('/')` passes locally against the real backend. Delete it afterwards: no reachability tests survive.

### Phase 2: The first seeded spec (end-to-end proof of the whole pattern)

Pick the first `{{FIRST_JOURNEYS}}` item that touches a third party.
1. **Backend side** (in the backend repo, own branch, never merged by you): a setup seed returning the ids the spec needs; a drain or transition seed if async; and the seed's TS type in `data/types/seeds.ts`.
2. **Mock side** (in `{{MOCKS_HOME}}`): vendor routes from the vendor docs, dynamic responses, reachable via env from the backend.
3. **Test side:**
   - the page objects;
   - a `<vendor>Mock.ts` verifier (entity match, status assert, poll until all expected calls, plus a negative counterpart);
   - the spec in A5 shape, asserting the primary UI end state **and** the outbound call.
4. **Prove it red.** Break the behaviour, show the failure for **each** assertion independently, then restore. Use a local revert or a toggled seed or mock; never commit the break. Record the red output.
5. Write the folder `CLAUDE.md` files for every folder touched (A10).

**Done-check:**
- the spec is green 5/5 locally with `--repeat-each=5`;
- there is recorded red evidence per assertion;
- no locators and no comments in the spec;
- the seed type matches the seed's JSON;
- every touched folder's `CLAUDE.md` is updated;
- a PR is open for each repo touched, cross-linked; backend and mock changes are never folded into the test repo commit.

### Phase 3: CI (minimal first)

**Minimal:**
- One reusable workflow, called by:
  - (a) PRs: `@smoke`, native `--shard=i/{{SMOKE_SHARDS}}` (2-3 is a good start);
  - (b) PRs into `{{RELEASE_BRANCH_PATTERN}}`: the full suite with native `--shard=i/{{FULL_SHARDS}}`.
- Services: DB, cache and the backend artifact from B6, **pinned**, booted with the test-mode env including the B4 flag and the mock URLs from B5.
- Steps:
  1. check out the mocks home and start each mock, then wait for health;
  2. install, then `playwright install chromium`;
  3. run `playwright test --project=chromium ...`;
  4. upload `blob-report/` per shard.
- Add a `merge-reports` job (`if: always()`) that runs `npx playwright merge-reports --reporter html` and uploads the HTML.
- Add a `skip tests` label escape hatch.

**Done-check:**
- a PR run shows every shard green, with the merged HTML report downloadable;
- a deliberately broken assertion on a scratch branch turns CI red with a usable trace; then delete the branch;
- the backend artifact reference in the workflow is a digest or version, not a floating tag.

**Full (Phase 4, each add-on only on demonstrated need; record the reason in the PR):**
- approval-label gating of smoke (document that "skipped" checks are not green);
- report publishing to Pages, plus a scheduled prune with dry-run default, a never-prune list and a refusal on truncated tree listings;
- runtime-balanced sharding: a reporter that sums per-file durations, a splitter that walks the tests dir on disk and greedy-LPT packs by weight with a median fallback, and a refresh workflow that opens a draft PR. **Only add this if native sharding is measurably skewed.** The reference built it and its timings file stayed empty, so it ran as round-robin;
- diff-scoped smoke tags (path glob -> tag map; fail open to `@smoke`). Only once there are feature tags to map;
- e2e coverage instrumentation, plus a CI-only warm-up in `globalSetup` if the instrumented cold compile threatens the first test's timeout;
- the B9 integration project workflow, label-gated;
- the B10 prod smoke workflow on deploy.

### Phase 4: Docs and the CLAUDE.md layer

1. Write the root e2e `CLAUDE.md` with every section A10 lists.
2. Ensure every folder has its `CLAUDE.md` with the standard footer.
3. Add the A6/A7 rules to the repo's main agent-instructions file and to any editor rule files the team uses, in the same wording. The reference kept a short "Testing Rules" section in the repo root `CLAUDE.md` and the full "What to Assert" list in an always-applied editor rule file.
4. **Use the kit's `claude-md-bundle/`** as shape examples: the root e2e file, `support/`, `pageObjects/`, `tests/` and the data-layer files, plus `editor-rules/`. `claude-md-bundle/MANIFEST.md` lists each file's tier. Adapt their structure and generic rules; do not copy reference content into the new project (its domains, seeds, vendors). The bundle has already replaced those with placeholders and neutral names; resolve every `{{placeholder}}` from Phase 0, and rename files from `CLAUDE.tmpl.md` to `CLAUDE.md`.

**Done-check:**
- a script (`find <e2e-root> -type d | while read d; do test -f "$d/CLAUDE.md" || echo MISSING $d; done`) prints nothing;
- every `CLAUDE.md` ends with one of the two footers;
- the root file's fixture list matches `support/fixtures.ts` exactly. Verify by grep, not by eye;
- `grep -rnE '\{\{[A-Z0-9_]+\}\}' <e2e-root>` finds no unresolved placeholder.

### Phase 5: Agent skills (workflow layer)

**Use the kit's `skills/` and `agents/`.** Port each by keeping its **generic workflow** and re-binding its placeholders to the Phase 0 answers. Do not wait for them to start authoring tests; they encode the workflows below, which you should already follow.

| Skill in this kit | Keep (generic) | Re-bind |
|---|---|---|
| `new-test` | Intake -> investigate-before-scaffold -> scaffold -> review; A5-A7 rules as numbered rules; the superficial-test hard rule | Seed paths, drain helper, vendor names |
| `running-e2e-tests` | Boot the frontend with CI's exact env; one run per frontend port; never kill a run you didn't start; readiness polling without sleeps | Host shell, ports, browser provisioning |
| `starting-the-e2e-backend` | One isolated backend per consumer (own DB, cache and port derived from a unique key); seed-endpoint failure taxonomy; leave as found | Boot commands (B1, B4, B13) |
| `investigating-e2e-failures` | Start from report/trace; localise the layer in order (browser -> seed/login -> app load -> assertion); classify (regression / stale selector / backend contract gap / seed-data / flaky / local-env mismatch); `--repeat-each=5`; never fix with a timeout or sleep | Commands |
| `reviewing-e2e-coverage` + the report-only agent `senior-qa-e2e-reviewer` | Rubric [Presence-only] / [Shallow] / [Deep]; all-effects; own-API read = Shallow; round-trip = blind; fixture-can't-produce = gap; full CRUD; report shape: sources -> feature model -> per-spec audit -> coverage matrix -> prioritised backlog (WHAT / WHERE / ASSERTION / SEED) -> gaps. Guardrails: no edits, no git writes, tickets read-only. | Intent source (B11) |
| `qa-e2e-test-only-changes` | Never edit product source to pass a test; document app fixes for devs; test-side is not shortcut-side | none |
| `editing-backend-seeds-and-mocks` | Shared checkouts need a coordinator ack before any mutation; authoring happens on your own branch or worktree; never mix repos in one commit; mocks are born in the mocks home; restart and re-prove; seed and mock authoring rules (A8, A9) | Repo paths |
| `testing-a-feature-end-to-end` | One worker runs the pipeline: intake -> intent -> plan (consolidate, full CRUD, full effect-set) -> backend/seed/mock -> author -> assertion depth -> run/triage -> bug hunt (fixme + finding) -> PR (never merge) -> independent review loop -> **independent interaction and branch completeness check** -> report -> file bugs. It uses the flat no-comment rule (A7). | Tracker, intent source |
| `analyzing-test-tickets` | Ticket -> neighbourhood (relations, siblings, open bugs in the area, linked tickets' ACs) -> intent -> discussion -> a Feature Analysis Report with suggested scenarios | Tracker/chat tools |
| `sweeping-releases-for-e2e-coverage-gaps` | Fetch first and state the SHA; **enumerate shipped work from the git diff of product source between release tags, never from the tracker**; trackers for intent only; exclusions recorded with a reason; cost sorts, never filters | Tag scheme |

**Done-check:**
- each ported skill exists with no unresolved `{{placeholder}}` left;
- a fresh worker given only "add a spec for `<journey>`" follows A5-A7 without being reminded. Verify by reviewing its PR against the A6/A7 checklist.

### Standing lessons (carry these into every phase)

The kit's `03-testing-lessons.md` expands each of these.

- **Isolation:**
  - Two runs against one backend corrupt each other through the per-test reset, so prove no other run is live first.
  - Derive per-worker ports from a unique key; never pick "free-looking" ports.
  - A killed run can leave in-process async jobs that deadlock the next run, so restart the backend.
  - Editing files under the dev-server root mid-run hot-reloads the page and destroys the test's context.
  - An in-process job adapter makes "job running" UI states unreachable; use `fixme` there, never green-by-presence.
  - Sleepy fan-out jobs can saturate an in-process pool and leak into the next test after id reuse, so drain each fan-out.
- **CI forensics:**
  - CI tests the PR **merge ref**, and a rerun replays the stale merge SHA. A base fix needs a fresh run.
  - "Skipped" checks are not green.
  - Early-stopped shards fake "not run" blocks in run diffs.
  - The merged report decodes per-spec outcomes per attempt, and the failing spec can swap between attempts. Keep published per-attempt reports.
  - A hung action is the trace's `before` event with no `after`.
  - "Flaky" or "CI infra" is not an accepted root cause for a recurring failure. Check history and backend changes; refute a candidate cause by its other necessary consequences; with two simultaneous changes, find a straddling window.
- **Release blocking:** when tests block a release, the first PR out is a `fixme` skip on the release branch. The fix comes after.
- **Proof:**
  - A pre/post-fix proof is only valid against committed bytes; compare blob hashes and re-run after any edit.
  - A long-skipped spec may assert behaviour the product deliberately changed; date the skip before calling it a bug.
- **Severity:** a synthetic fixture built to maximise differences inflates how wrong output looks. Re-measure on a realistic record before escalating.
- **Scope:** QA writes e2e, not backend unit tests. An async end state that genuinely has no UI surface goes to developers as a coverage note.
- **Coding:**
  - Dead code found while fixing is removed in the same PR.
  - Bug-hunt reports list application bugs only; test-automation defects are fixed and noted in the changelog.
  - Prefer structural enforcement (self-scoping projects, lint rules) over a flippable flag plus documentation.
