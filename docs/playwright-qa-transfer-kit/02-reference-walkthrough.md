# Reference implementation walkthrough: a production Playwright e2e setup

This document describes, layer by layer, the Playwright suite the transfer prompt (`01-transfer-prompt.md`) was distilled from. Read it when the prompt's terse rule is not enough and you need the reasoning, the mechanism behind a convention, or the place where the reference itself fell short.

Conventions in this document:
- The reference is a React single-page app (the "frontend") talking to a Rails API (the "backend"), with a separate repo holding HTTP mock environments for third parties. Its product domain is replaced throughout by neutral names: the main entity is an **item**, items are grouped in **bundles**, users belong to a **tenant**, a person acting in the app is a **member**, a domain record is a **record**, and outbound third-party targets are **destinations**. The two third parties are `{{VENDOR_A}}` (a distribution API the backend pushes updates to) and `{{VENDOR_B}}` (an upstream system-of-record vendor that feeds an integration pipeline).
- Paths are repo-relative paths "in the reference project", with domain folders renamed the same way (`tests/main/item/`, `pageObjects/pages/Item/`). No line numbers are given: the reference repo is private.
- **[HYPOTHESIS]** marks something inferred rather than read.

---

## 0. One-paragraph model of the system

The suite is a single-browser (chromium) Playwright suite. It drives the real React SPA (Vite dev server on `{{FRONTEND_PORT}}`) against a real backend running in **test mode** (Rails `RAILS_ENV=test` on `{{BACKEND_PORT}}`). Before **every** test, an auto-fixture does four things:
1. Truncates the whole backend DB through a test-only HTTP endpoint.
2. Purges the transaction logs of the HTTP mock servers (Mockoon) that stand in for third parties.
3. Creates a super-admin.
4. Plants that admin's bearer token in `localStorage`.

Each spec then asks the backend for its domain state by **name**: `server.seed('seeds/main/<scenario>')` runs a committed Ruby file on the backend and returns JSON ids. The spec then drives the UI only through Page Objects and asserts the end state the user sees. Every outbound third-party call is asserted at the mock's transaction log.

CI runs this against a prebuilt backend Docker image. It runs `@smoke` on approved PRs (a few shards) and the full suite on PRs into the release branches (more shards, file lists from a runtime-weighted splitter). It merges blob reports into one HTML report and publishes it to GitHub Pages.

Around this sits a `CLAUDE.md` in every folder of the suite: the per-folder rulebook. About ten user-level agent skills encode the authoring, run, triage, review and sweep workflows. This kit carries both, templated.

---

## 1. Layer 1: Config

### 1.1 `playwright.config.ts`

| Setting | Value | Note |
|---|---|---|
| env file | `dotenv.config({ path: './playwright/.env' })` | The file is gitignored and no `.env.example` is tracked. The new project should commit one. |
| `testDir` | `./playwright/tests` | |
| `fullyParallel` | `true` | |
| `forbidOnly` | `!!process.env.CI` | |
| `retries` | CI `2`, local `0` | |
| `workers` | **`1` on CI and `1` locally** | See 1.1(b). |
| `reporter` | CI: `blob`, plus a custom timing reporter when a collect-timings env var is set. Local: `html`. | |
| test `timeout` | CI 60 s, local 40 s | |
| `expect.timeout` | CI 60 s, local 20 s | |
| `actionTimeout` / `navigationTimeout` | **not set**, so Playwright defaults apply (0 = bounded only by the test timeout) | |
| `maxFailures` | CI `3` (per shard invocation), local unlimited | Poisons run-to-run diffs; see 4.4. |
| `use.baseURL` | the frontend URL, hard-coded | |
| `use.trace` | `retain-on-failure` | |
| `storageState` | **none** | Auth is done per test by the fixture (3.4), not by a setup project or storageState file. |
| `globalSetup` | `support/global-setup.ts` | On CI only, it warms two routes once, because the coverage-instrumented cold compile is slow. |
| project `chromium` | Desktop Chrome, `--disable-web-security`, `en-US`, a larger viewport on CI than locally, `testDir ./playwright/tests`, `testIgnore '**/tests/integration/**'` | Deliberately fail-open. |
| project `integration` (exported constant) | `testDir ./playwright/tests/integration`, its own `baseURL` | Self-scoped side project for the multi-service harness. |
| `webServer` | `{{BACKEND_URL_ENV}}=${...:-default} yarn start`, waits on the login URL, 120 s, `reuseExistingServer: !CI` | CI therefore has Playwright boot Vite itself. |

Two design choices in this config are load-bearing.

**(a) Fail-open project scoping.** Only the integration project self-scopes; `chromium` collects everything else. A spec placed in a new folder still runs. Narrowing `chromium` to named folders would silently orphan specs and still exit 0. The suite's `tests/CLAUDE.md` makes it a rule: adding a second self-scoped project means adding its path to chromium's `testIgnore` in the same change.

**(b) `workers: 1` despite `fullyParallel: true`.** The reason is not stated in the config. **[HYPOTHESIS]:** the global `seed('clean')` truncation (3.3) would let parallel workers truncate each other's data mid-test; two runs sharing one backend were observed doing exactly that. CI parallelism therefore comes from **shards (separate runners, separate DBs)**, never from workers.

### 1.2 `{{PROD_SMOKE_CONFIG}}`

A separate config for a read-only production smoke test. The main config never collects it.

| Setting | Value |
|---|---|
| `testDir` | `./playwright/prodSmoke` |
| `fullyParallel` / `workers` | `false` / `1` |
| `retries` | `1` (always, also locally) |
| `reporter` | CI `list` + `html` (open never); local `list` |
| `timeout` / `expect.timeout` | 90 s / 30 s |
| `baseURL` | `{{PROD_SMOKE_TARGET}}` (env-overridable) |
| `trace` / `screenshot` | `retain-on-failure` / `only-on-failure` |
| project | one, `prod-smoke`, Desktop Chrome, `en-US` |
| `globalSetup`, `webServer`, fixtures | **none**: no backend, no Vite, no `support/fixtures` |

CI runs it with `playwright test -c {{PROD_SMOKE_CONFIG}}`, triggered by `repository_dispatch` (fired after a backend production deploy) and `workflow_dispatch`.

### 1.3 `playwright/tsconfig.json` and aliases

- `playwright/tsconfig.json` extends the repo `tsconfig.json`, with `baseUrl` pointing at the app source, `strict: true`, `noEmit`, `types: ["node"]`, `useUnknownInCatchVariables: false`, and `include ["./"]`.
- Aliases come from a shared `tsconfig.paths.json`: one alias for the legacy Cypress tree, one for `playwright/`, and `~/*` for the app source.
- The suite's root `CLAUDE.md` documents the **number-one trap**. Importing an app barrel (an index file that re-exports an env-config module) from a page object or spec makes Playwright's loader fail with "exports is not defined in ES module scope". Every spec in that import chain then reports **"No tests found"**. The suite therefore keeps its own dep-clean util leaf, `playwright/support/utils.ts`.
- Seed result types live in the **legacy Cypress tree** (one large `seedTypes.ts`). The new project should put them in `<e2e-root>/data/types/seeds.ts` instead and keep the "never inline" rule.

### 1.4 package.json scripts and deps

- **No `playwright`/`e2e` npm script exists.** Everything is invoked as `yarn playwright test ...` (the bin). The only Playwright script is a Prettier pass over `playwright/**`.
- `@playwright/test` is pinned to an exact version. Cypress and its plugins are still present, and Cypress is deprecated for new work.
- `yarn lint` covers only `./src`. **Nothing in CI lints, formats or type-checks the Playwright suite**, apart from the transpile Playwright itself does at run time. Every "no locators in specs" and "no comments" rule is therefore enforced by docs and review only, and dead module variables survive in specs. The transfer prompt makes the lint backstop mandatory for this reason.
- nyc excludes `playwright/**` from coverage. Coverage measures app source as exercised by the e2e run (4.6 of the CI layer).
- Local toolchain: a repo `flake.nix` pins node and yarn. The Playwright devshell with a matching chromium lives outside the repo, per host (`{{DEVSHELL}}`).

---

## 2. Layer 2: Folder architecture

### 2.1 Tree and roles

```
playwright/
|-- CLAUDE.md                suite index + root conventions
|-- tsconfig.json
|-- reporters/spec-timing-reporter.ts   per-file duration -> {{TIMINGS_FILE}} (one partial per shard)
|-- support/                 ALL infra; the ONLY home for fixtures/helpers
|   |-- fixtures.ts          the single shared `test` (auto forEachTest + named fixtures)
|   |-- server.ts            Server: seed(), runScheduler(), lifecycle polls, VCR control
|   |-- backendApi.ts        dep-clean primitives: postCommand, unwrapSeedResult, graphqlQuery, signInForBearer
|   |-- userFlows.ts         login / tenant switch / role users / clearSession
|   |-- mockClient.ts        dep-clean Mockoon admin client (purgeLogs/purgeState/getLogs/waitForLogs)
|   |-- vendorAMock.ts       vendor-A assertions (verifyVendorReceived*)
|   |-- vendorBMock.ts       vendor-B assertions (expect*), integration harness
|   |-- integration*.ts      drivers for the multi-service integration project
|   |-- projects.ts          project-name consts
|   |-- global-setup.ts      CI-only warm-up
|   `-- utils.ts             suite-own util leaf (formatters, oracles, upload paths)
|-- pageObjects/
|   |-- BasePage.ts          base class every PO extends
|   |-- pages/<Domain>/      one class per route/screen (plural = list, singular = detail)
|   `-- components/          drawers/ modals/ sections/(list = table machinery) headers/ sidebar
|-- data/
|   |-- factory/             faker builders that fill FORMS (not backend rows)
|   |-- models/              TS types + preset constant arrays
|   `-- fixtures/            static GraphQL mock payloads (Responses/), constants, a binary video
|-- tests/
|   |-- account/ auth/       ungated surfaces (chromium)
|   |-- main/<domain>/       the main suite (chromium), by product domain
|   `-- integration/         multi-service harness (`--project=integration`)
`-- prodSmoke/               production smoke, own config
```

The suite holds on the order of a hundred spec files, about two thirds of which carry at least one `@smoke` test, and one `CLAUDE.md` per folder.

### 2.2 Placement and naming rules

- **Domain folders.**
  - Specs go in `tests/main/<domain>/`, ungated shared surfaces in `tests/account|auth/`, and the integration flows in `tests/integration/`.
  - One domain is **frozen**: every describe is `test.describe.skip`, and its folder doc says not to add or un-skip specs there. That is the sanctioned way to park a whole area.
- **No `support/` under `tests/**`.** There is one fixture object and it lives in `support/fixtures.ts`.
- **Page objects:**
  - Each extends `BasePage` and is a `default` export.
  - Selectors are **private getters**; methods take arguments only when needed, e.g. a row id.
  - Navigation methods **return the next PO**.
  - Plural filename = list page, singular = detail page.
- **PO registration.** POs used across many specs are registered as named fixtures in `support/fixtures.ts`. Others are `new X(page)` inline. Rule: destructure the fixture you need; never construct a second copy.
- **Data:**
  - **A factory is not a seed.** Factories build form input with faker and never touch the backend.
  - Persistent state comes only from backend seeds.

### 2.3 Representative spec walked end to end: `tests/main/item/stopDestination.spec.ts`

This spec touches every layer: a named seed, a tenant switch, the POM, a UI action, the async drain, a UI poll, and third-party log assertions. The authoring and review skills cite it as the reference pattern.

1. **Imports.** Seed types come from the shared seed-types file, and `test` from `support/fixtures`, never from `@playwright/test`.
2. **Auto fixture, before the spec's own code** (`support/fixtures.ts`, `{ auto: true }`):
   1. `new Server().seed('clean')` -> `backendApi.postCommand(ctx, BACKEND_URL, 'clean')` -> `POST {BACKEND_URL}/__cypress__/command` with `{name:'clean', options}` -> backend `spec/cypress/app_commands/clean.rb`. That runs a DatabaseCleaner truncation of the whole DB, ejects and turns off VCR, resets OmniAuth mocks and disables WebMock.
   2. Vendor-A mock: `purgeLogs()`, which is `POST {base}/mockoon-admin/logs/purge`. It is a no-op when the vendor-A mock base-URL env var is unset.
   3. Vendor-B mock: `purgeState()` (`PURGE {base}/mockoon-admin/state`) plus `purgeLogs()`.
   4. `new UserFlows(page).loginWith()`:
      - `Server.createAdmin()` -> a `factory_bot` command creating a user with `super_admin: true` and a default language;
      - `signInForBearer` -> `POST {BACKEND_URL}/users/sign_in`, which reads the `Authorization` header;
      - `ensureOriginContext()` routes a fake `/__pw_setup` page so `localStorage` is writable on the app origin. It also pre-rejects the cookie-consent banner (`{{CONSENT_STORAGE_KEY}}`);
      - `localStorage.setItem({{TOKEN_STORAGE_KEY}}, token)`.
3. **`beforeEach`**:
   1. `server.seed('seeds/main/item_bundle_live')` -> a backend seed file of the same name. That seed:
      - enables feature flags first;
      - builds tenant, member, record, assets, bundle and item with FactoryBot;
      - forces a deterministic *live + published* state with `update_columns`;
      - returns `{tenant, record, item, bundle}.to_json`.
      `unwrapSeedResult` strips cypress-on-rails' one-element array and re-parses the JSON string.
   2. `userFlows.changeTenantTo(seeded.tenant)` sets `{{TENANT_STORAGE_KEY}}` in `localStorage`.
   3. `page.goto(itemsPage.url)` (`/main/items`).
   4. `itemsPage.validateVisible(false)` asserts the non-empty list state. If a drawer overlay is stuck, it reloads first.
4. **Test body**:
   1. `waitForItemPhase(item, 'live')` -> a table helper that polls the row's phase column with reload. The display string is derived by a util in `support/utils.ts`, not hard-coded.
   2. `pauseItem(item)`:
      - registers `waitForGraphQLResponse('mutation setItemPhase')` **before** the click (from `BasePage`);
      - clicks the row's Pause action through the table component;
      - awaits the mutation;
      - asserts the success toast. The toast is a *secondary* check.
   3. `server.runScheduler()` -> transition seed `seeds/transitions/run_schedulers`, which calls `perform_now` on the scheduler jobs. This deterministically drains the async work the click enqueued.
   4. `waitForItemPhase(..., 'paused')` checks the **primary state change** as the user sees it.
   5. `vendorAMock.verifyVendorReceivedPauseUpdate({ itemUuid, destinations })` checks the **downstream third-party effect** at the vendor-A mock's transaction log, per destination path. The fixture is `new VendorAMock({{VENDOR_A_MOCK_URL_ENV}})`.
   6. The same steps run again for resume -> `'live'` + `verifyVendorReceivedActiveUpdate`.
5. **Teardown.** After `use()`, the fixture collects `window.__coverage__` into `.nyc_output/` when the coverage env flag is set. Nothing else is cleaned up after the test: isolation is *clean-before*, not *clean-after*.

### 2.4 BasePage services

| Area | Methods | Notes |
|---|---|---|
| GraphQL waits | `waitForGraphQLRequest` / `waitForGraphQLResponse(operationName)` | Gate an action on a named operation. Specific to a GraphQL app. |
| Floating-UI stability | `openUnstableFloatingSection`, `waitForDrawerClosedOrReload` | Specific to the app's UI library (Headless UI portals); the first may reload a few times. |
| Multi-element waits | `waitForAnyElementToBeVisible/Attached` | Generic. |
| Async write-back | `reloadUntil(assertion, timeout)` | `expect(...).toPass` with reload. Generic. |
| Query intercepts | `intercept*Query` | Fulfil canned `data/fixtures/Responses/*` so specs never hit an external estimation service that is not under test. |

---

## 3. Layer 3: Test data and seeding

### 3.1 The mechanism: cypress-on-rails

- **Gem:** `cypress-on-rails` in the backend Gemfile.
- **Initializer:** `config/initializers/cypress_on_rails.rb`:
  - `cypress_folder = spec/cypress`;
  - `use_middleware = !Rails.env.production?`;
  - `use_vcr_middleware = !Rails.env.production?`;
  - an explicit "can execute arbitrary ruby" warning.
- **Endpoint:** `POST /__cypress__/command` with body `{name, options}`. It runs `spec/cypress/app_commands/<name>.rb` with `command_options` bound. The `.rb` file's last expression is the response, wrapped in an array.
- **Readiness/404 semantics** (documented in the backend-start skill):
  - a JSON 404 `could not find command file` means the endpoint is up but the name is wrong;
  - an HTML 404 means it is not mounted (wrong env);
  - a 500 means the seed raised.
- **Loaded once:** `spec/cypress/cypress_helper.rb`. It requires DatabaseCleaner and FactoryBot, configures a factory wrapper over the backend's factory folders, and configures VCR with `ignore_localhost = true`, so localhost mocks are real servers, not recordings.
- **Generic commands** (`spec/cypress/app_commands/`):
  - `clean.rb` (truncate + VCR off + WebMock off);
  - `factory_bot.rb` (arbitrary FactoryBot calls);
  - `eval.rb` (`Kernel.eval(command_options)`, **banned** from the Playwright suite by its root `CLAUDE.md` and the `new-test` skill);
  - `log_fail.rb`.
- **VCR:** `/__cypress__/vcr/insert|eject` are wrapped by `Server.insertCassette/ejectCassette`. They exist mainly for the legacy Cypress suite, and VCR specs need a single-threaded backend.
- **Test-env job adapter:** the backend's `config/environments/test.rb` sets `queue_adapter = ENV['{{ASYNC_TEST_FLAG}}'] ? :async : :test`, and `enqueue_after_transaction_commit = :always` under the same flag. The backend therefore must be booted with that flag set, as CI does. The `:async` adapter is in-process and **blind to job-status tracking and the real job queue**, which makes "job running" UI states unreachable.

### 3.2 Seed inventory and types

- **Seed folders** under `spec/cypress/app_commands/seeds/`: one large folder per product area (the main area and a newer app-wide folder hold most of them), `integration/`, `transitions/`, the report app, the frozen domain, plus a handful of legacy top-level files. The Playwright suite calls seeds from the two big area folders and from `transitions/`.
- **Seed kinds**, as the reference's seed-authoring guide distinguishes them:
  1. **Setup seeds** build a scenario and return the ids/tokens a spec needs, e.g. `main/item_bundle_live`.
  2. **Transition seeds** live under `transitions/` and do *state advancement only*: `run_schedulers`, `make_item_live|finished`, `prepare_bundle`, `invoke_distribution`, and `get_item_status`, a read-only status oracle returning `{phase, published_at}` that is used for *waiting*, never as the assertion.
  3. **Generic commands:** `clean`, `factory_bot`.
- **Authoring rules:**
  - `# frozen_string_literal: true`;
  - feature flags **before** data;
  - FactoryBot + traits only;
  - navigate associations, no orphans;
  - comment any `update_columns`/`validate: false` bypass;
  - params via `command_options` (`fetch` for required);
  - return a **minimal** hash via `.to_json`;
  - use the splat-and-extend pattern to include associations.
- **Seed types:** the TS shape of every seed's JSON goes in the shared seed-types file, never inline.
- **Calling gotcha:** call `server.seed(name, options)` **with** an options object whenever the seed reads one. Omitting it sends no `options` key, so `command_options.fetch(...)` raises on nil and the seed 500s.
- **Failure messages name the command.** `postCommand` throws `Failed to execute command: <name>`, because seeds are the top source of e2e failures.

### 3.3 Isolation between tests

- **Strategy: global truncate before every test, plus serial execution.**
  - Every test starts from an empty DB (`clean.rb`) with a freshly created admin. Mock logs are purged and the vendor-B mock state is reset.
  - There is no after-test cleanup.
  - CI isolates shards by giving each shard its own runner, Postgres service and backend container.
- **Consequences:**
  - Two runs sharing one backend truncate each other.
  - A killed run leaves `:async` jobs that deadlock the next run.
  - Ids are reused after `clean`, so a prior test's late async call can match the current test's log window. That is why the purge is per test and why verifiers match on specific entity ids.
- **Locally, every consumer gets its own backend** (own worktree, Postgres, Redis, a DB name derived from the worker's slug, a port hashed from the slug into a fixed range). There is no shared e2e backend. The `starting-the-e2e-backend` skill encodes this.

### 3.4 The auth shortcut (`sign_in`)

- There is no UI login per test.
  - `signInForBearer` posts to `/users/sign_in` and takes the `Authorization: Bearer` header.
  - `UserFlows` writes it to `{{TOKEN_STORAGE_KEY}}` on the app origin, using the `/__pw_setup` route-fulfil trick to get an origin without loading the app.
- **Role and tenant variants:**
  - `createUserWithRole(role, tenantUuid?, email?)` -> a user seed;
  - `loginTenantUserWithRole`;
  - `assignUserToTenants`.
- **Admin panel (server-rendered, backend origin):** `signInAsAdmin()` -> a passwordless `GET` route on the backend. It only works when the backend runs with `{{REVIEW_APP_FLAG}}` set, which CI sets.
- **`clearSession({cookies: true})`** is mandatory in specs that then sign in through the UI with credentials. Otherwise a surviving session cookie carries the sign-in, and a "these credentials no longer work" assert passes for the wrong reason.
- **Two gotchas:**
  - `sign_in` answers 201, not 200;
  - a reused `APIRequestContext` keeps the session cookie, so later sign-ins "succeed" with any password.

### 3.5 Mock servers and asserting third-party calls at the transaction log

- **Mockoon** (a pinned `@mockoon/cli` in CI) serves JSON environments from a **separate mocks repo**:
  - the vendor-A environment on `{{VENDOR_A_MOCK_PORT}}`;
  - the vendor-B environment on `{{VENDOR_B_MOCK_PORT}}`, integration project only.
- **Wiring:** the backend's outbound base URL is pointed at the mock by env (`{{VENDOR_A_OUTBOUND_URL_ENV}}=http://host.docker.internal:{{VENDOR_A_MOCK_PORT}}` in CI). The test side addresses each mock by its base-URL env var. The admin path `/mockoon-admin` is a constant.
- **Client contract** (`mockClient.ts`):
  - an unset base URL makes purges no-op and reads throw;
  - `fetchAllLogs` paginates 100 per page with a page-count runaway backstop that throws rather than truncates;
  - `waitForLogs(predicate, {minMatches})` waits for **all** N expected calls, so exact-count asserts can't race;
  - reads are not time-filtered, to avoid clock skew.
- **Assertion helpers:**
  - **Vendor A** (`vendorAMock.ts`):
    - `verifyVendorReceived{Archive,Pause,Active}Update`;
    - `verifyVendorReceivedRedistribute` and its negatives `...DidNotReceiveRedistribute` / `...ReceivedNoRedistribute`;
    - a few further per-payload verifiers, each with a `{,No}` negative or `...AttemptFailed` variant.
  - **Vendor B** (`vendorBMock.ts`): `expectRecordResolved`, `expectEntriesResolved`, `expectOrderStatusPosted`, `expectRecordAttachmentsFetched`, all through one `expectCalls(method, path, {status, times})`.
- **Rules encoded in these helpers** (from `support/CLAUDE.md`):
  - every verifier asserts the **response status**, so a 404 fall-through fails instead of greening on a mere hit;
  - **match on the specific entity** (uuid, name, actor id), not the path alone;
  - **poll until every expected key arrives**, because destinations post at different times;
  - every positive helper has a **negative counterpart**;
  - **extend** an existing helper with an optional param rather than adding a sibling;
  - if a vendor path isn't modelled, **add a verify method**; never drop the assertion.
- **Mock authoring rules** (from the seeds-and-mocks guide):
  - routes come from the **vendor's own API docs**, cited in the route's `documentation` field;
  - responses are **dynamic** (echo request values, templating enabled, valid JSON);
  - if our code diverges from the vendor contract, flag it as a bug instead of bending the mock;
  - new mocks are born in the mocks repo and are never vendored into the frontend.

---

## 4. Layer 4: CI

### 4.1 Workflow map

| Workflow | Trigger | Gate | What it runs |
|---|---|---|---|
| `{{APPROVAL_WORKFLOW}}` | `pull_request_review: submitted` | at least 1 approval | Adds label **`approved`** |
| `{{SMOKE_WORKFLOW}}` -> `compute-tags` + `smoke-test` | PR `labeled/reopened/synchronize`, **not** into the release branches | has `approved` AND not `skip tests` / `{{LEGACY_RUNNER_FULL_LABEL}}` / `{{FULL_REGRESSION_LABEL}}` | `{{DIFF_TAGS_SCRIPT}} --base-tag=@smoke` -> base workflow with `grepTags` (smoke shard count) |
| `{{SMOKE_WORKFLOW}}` -> `full-test` | same | label `{{LEGACY_RUNNER_FULL_LABEL}}` and not `skip tests` | base workflow, full |
| `{{SMOKE_WORKFLOW}}` -> `playwright-full` | same | label `{{FULL_REGRESSION_LABEL}}` and not `skip tests` | base workflow, full |
| `{{FULL_REGRESSION_WORKFLOW}}` | PR `opened/reopened/synchronize` **into the release branches** | not `skip tests` | base, full, **`collectTimings: true`** |
| `{{RELEASE_TESTS_WORKFLOW}}` | PR labeled, not into release branches | `{{RELEASE_TESTS_LABEL}}` and not `skip tests`; concurrency cancel-in-progress | base full + timings, **and** full Cypress |
| `{{INTEGRATION_WORKFLOW}}` | PR labeled/sync, not into release branches | label `{{INTEGRATION_LABEL}}` | Full multi-service stack (backend image + integration gateway + message queue + S3 emulator + vendor mocks + Vite), `playwright test --project=integration` |
| `{{TIMINGS_REFRESH_WORKFLOW}}` | `workflow_run` completed of the full-suite workflows, or `workflow_dispatch(runId)` | success on a release branch, or the label run | Downloads per-shard timings, merges, opens a **draft PR** to `{{DEFAULT_INTEGRATION_BRANCH}}` if they drifted |
| `{{REPORT_PRUNE_WORKFLOW}}` | weekly cron + dispatch | dispatch defaults `dry_run: true` | Deletes gh-pages report dirs older than 30 days. Protects `.github`, `.gitignore`, `.nojekyll`. Refuses if the tree listing is truncated. |
| `{{PROD_SMOKE_WORKFLOW}}` | `repository_dispatch` (post backend prod deploy) + dispatch | none | `-c {{PROD_SMOKE_CONFIG}}` |
| `<sibling-app>-release-gate.yml` | PR into release branches + dispatch | not `skip tests` | Dispatches the sibling report app's own Playwright run in its repo and polls it (cross-repo gate) |

### 4.2 The reusable base: `{{BASE_WORKFLOW}}`

- **Inputs:** `grepTags` (empty = full run), `collectCoverage` (default **true**), `collectTimings` (default false). Secrets: a repo token, a registry token, the backend secret key, and the frontend's third-party API tokens.
- **Matrix:** a small shard count when `grepTags` is set, a larger one otherwise. `fail-fast: false`, `timeout-minutes: 60`.
- **Service containers:**
  - `postgres` (pinned major);
  - `redis`;
  - **the backend image `{{CI_BACKEND_ARTIFACT}}`** on a floating tag, with:
    - `RAILS_ENV=test`, `{{ASYNC_TEST_FLAG}}=true`, the legacy Cypress flag, `{{REVIEW_APP_FLAG}}`;
    - single-threaded Puma (min/max threads 1, no workers);
    - the job-queue Redis URL set explicitly, because the image bakes a localhost default for any variable the workflow forgets;
    - `{{VENDOR_A_OUTBOUND_URL_ENV}}=http://host.docker.internal:{{VENDOR_A_MOCK_PORT}}`;
    - `--add-host=host.docker.internal:host-gateway`.
  - The DB name interpolates a matrix key that does not exist, so it resolves to a fixed string with a trailing underscore **[observation; harmless]**.
- **Job env:** `{{BACKEND_URL_ENV}}`, `{{FRONTEND_ENV_VARS}}` (map-tiles token, sibling-app URLs on staging), the coverage flag, `CI`, `BACKEND_URL`, `{{ADMIN_EMAIL}}`/`{{ADMIN_PASSWORD}}`, `{{VENDOR_A_MOCK_URL_ENV}}`.
- **Steps:**
  1. checkout with submodules (the UI kit);
  2. checkout the mocks repo into `mocks/`;
  3. `npm i -g @mockoon/cli@<pinned>`;
  4. `mockoon-cli start --data mocks/<vendor-A>.json --port {{VENDOR_A_MOCK_PORT}} &`;
  5. `wait-on` the mock's health route;
  6. package-registry auth;
  7. `yarn install --frozen-lockfile`;
  8. `yarn playwright install chromium`;
  9. run:
     - smoke: `yarn playwright test --project=chromium --grep "$grepTags" --shard=i/N`;
     - full: `node {{SHARD_SPLITTER_SCRIPT}} --shard=i --total=N` gives a file list, then `yarn playwright test --project=chromium <files>`; an empty list is a warning and exit 0;
  10. upload `blob-report/` as `playwright-report-<i>`;
  11. timings artifact when collecting;
  12. `.nyc_output/` coverage artifact.

  Vite is booted by the config's `webServer`, not by a workflow step.
- **`merge-reports` job** (`if: always()`):
  1. downloads `playwright-report-*` -> `npx playwright merge-reports --reporter html`;
  2. uploads `playwright-report-merged`;
  3. **publishes to gh-pages** under a unique per-run folder (timestamp, branch and a run id) with `keep_files: true`;
  4. writes the Pages URL to the step summary.

  The gh-pages copy survives re-runs that overwrite artifacts, which makes it the forensics record.
- **`code-coverage` job:** `nyc merge` + html/cobertura report, artifact, a PR comment, then it deletes the raw artifacts. A cleanup job handles the failure path.

### 4.3 Sharding and timings: designed vs actual

- **Designed:**
  - a `SpecTimingReporter` sums per-file durations;
  - shard partials are merged by `{{TIMINGS_MERGE_SCRIPT}}` (replace-with-merged + drift stats);
  - `{{SHARD_SPLITTER_SCRIPT}}` walks `playwright/tests` on disk, weights files from `{{TIMINGS_FILE}}`, falls back to the median weight (or a fixed default weight if there are no timings), and greedy-LPT bin-packs.
  - The splitter walks `tests/integration/` too. Those files occupy bin slots and contribute no tests under `--project=chromium`.
- **Actual:** `{{TIMINGS_FILE}}` is empty and has never been refreshed. Every file gets the same default weight, so LPT degenerates to **round-robin over the alphabetically sorted file list**. Adding or removing one spec file reshuffles every later file's shard.
  - **[HYPOTHESIS]:** the refresh workflow has opened draft PRs, but none was merged.
- **Diff-scoped tags (smoke).** `{{DIFF_TAGS_SCRIPT}}` unions `@smoke` with feature tags whose globs match the PR diff, and fails open to `@smoke`. The tag map contains only a comment, so the smoke grep is always exactly `@smoke`.

### 4.4 Smoke vs full regression

- **`@smoke`** = "runs on every PR". Untagged = full regression only.
- **Smoke** runs only after **one approval** (label `approved`). A PR whose smoke jobs show "skipped" has **never run its specs**.
- **Full regression** runs on PRs into the release branches and on labels (`{{FULL_REGRESSION_LABEL}}`, `{{LEGACY_RUNNER_FULL_LABEL}}`, `{{RELEASE_TESTS_LABEL}}`). **Observation:** `{{LEGACY_RUNNER_FULL_LABEL}}` also triggers a full *Playwright* run, while the suite doc describes it as Cypress only. Intent was not checked.
- **`skip tests`** suppresses every Playwright job.
- **CI tests the PR merge ref.** A rerun replays the recorded merge SHA, so a fix on the base branch needs a fresh run (close/reopen).
- **The backend under test is the floating backend image, not the PR's backend.** A broken backend integration branch reddens every frontend PR, and release reds must be A/B'd by **image digest**.
- **`maxFailures: 3` per shard** stops a shard early. Run-to-run diffs then show fake "passed -> not run" blocks.

---

## 5. Layer 5: Conventions and docs

### 5.1 Root `CLAUDE.md` testing content

- **"Testing Rules"** (a section summarising the testing editor rule):
  - unit tests are required for new or modified utils/hooks;
  - read "What to Assert" before writing assertions;
  - **"A test with no demonstrated pre-fix failure is superficial"**: run it red against the bug, split tests so each rule fails on its own, and deepen shallow asserts in the touching change;
  - Playwright with POM for new e2e, prefer `getByRole`/`getByTestId`; Cypress is deprecated.
- **"Documentation of New Patterns"**: every `playwright/` folder carries a `CLAUDE.md`. Update it in the same change; new folders get one with the `> **Maintenance:**` footer.

The kit carries this file as `claude-md-bundle/CLAUDE.tmpl.md`.

### 5.2 The testing editor rule (`.cursor/rules/testing.mdc`, `alwaysApply: true`)

- **"What to Assert (Required)"**, the normative core:
  - every AC, including linked tickets' ACs, gets a behavioural assertion; never existence or visibility;
  - every population-naming AC gets its complement;
  - **assert the rule, not the row**; a round-trip is correct only when user input *is* the feature;
  - a branch the fixtures can't reach is uncovered, so extend the fixture;
  - no demonstrated pre-fix failure means superficial;
  - an assertion behind an earlier aborting one is unproven, so split it;
  - the pre-fix-failure rule beats the complement rule;
  - deepen shallow asserts on touch.
- **Playwright section:** no locators in tests, private-getter locators in POs, `describe`/`beforeEach`/fixtures, isolation.
  - **Drift:** its PO example is a plain class that does **not** extend `BasePage`, while `pageObjects/CLAUDE.md` mandates `extends BasePage`. Fix that when you port it.
- **Generic boilerplate:** test data, cleanup, coverage.
- **Other editor rules that touch tests:** the documentation rule (per-folder `CLAUDE.md`; a new testing pattern goes into the testing rule) and the best-practices rule (points to the testing rule). The GraphQL and React Query rules do not touch e2e.

The kit carries these as `claude-md-bundle/editor-rules/*.tmpl.mdc`.

### 5.3 `playwright/CLAUDE.md`, the suite index

Its sections, in order:
- folder map;
- the frozen-domain notice;
- the two projects;
- prod smoke;
- run commands;
- the `~` alias trap;
- fixture wiring;
- **standard spec shape**;
- how seeds work;
- **nine suite-wide conventions**:
  1. assert the end state via the UI, never the app's API or the console;
  2. one action, many effects, assert all;
  3. every third-party touch at its mock log;
  4. no locators in specs;
  5. reuse before writing;
  6. no standalone page-loads test, merge shared-setup tests;
  7. **no comments**, a flat ban (never explain app source; provenance goes to the PR or ticket);
  8. `@smoke` semantics;
  9. "What to Assert" pointers;
- CI behaviour;
- cross-links;
- footer.

**Observed drift from rule 7:** the support files and some POs still carry explanatory comments. **[HYPOTHESIS]:** the no-comment rule post-dates them and is applied on touch.

### 5.4 Per-folder `CLAUDE.md` inventory

**Standard footers.** Every file under `playwright/` carries one of two variants (the repo root file carries none):
- leaf/content folders:
  > `> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.`
- index folders:
  > `> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.`

**Standard body shape:** title `# <path> - <role>` + parent link -> `## What's here` -> `## How to use it` -> `## Conventions & gotchas` -> `## Cross-links` -> footer.

The full inventory, with each file's tier (generic / mixed / project-specific) assigned after a full read, is `claude-md-bundle/MANIFEST.md`. In short: the mixed files at the top of the tree (the suite root, `support/`, `pageObjects/`, `tests/`, the main-area index, plus the repo root) carry nearly all the conventions. The generic ones are short templates. The project-specific ones transfer only as an **example of the shape**, so the kit reduces them to outlines.

---

## 6. Layer 6: Agent tooling around the suite

All of these were user-level files on the reference host, not in the repo. The kit carries them, generalized, under `skills/` and `agents/`.

| Skill / agent (kit name) | Workflow and rules encoded | Bindings the kit turned into placeholders |
|---|---|---|
| `new-test` | Interactive intake (feature, domain folder, smoke?) -> **investigate before scaffold** (seed JSON shape, POs, domain states, existing patterns) -> scaffold -> review with the user. A HARD RULE that superficial tests are worse than none, defined by five criteria. Then numbered rules: import from fixtures; seed types match JSON; assert every effect (toast secondary); `test.slow()` for lifecycles; no locators; **no comments**; real user flow, async = click + drain + poll; consolidate shared-setup tests and no page-loads test; reuse/extend, never fork; **no console/eval reads, the UI is the only oracle**; declarative spec body; cross-cutting cleanup in the global fixture, no reachability pre-checks; every third-party touch at its mock log; update the folder CLAUDE.md; assert the rule not the row. | Seed paths, drain helper, vendors, the integration project section. |
| `running-e2e-tests` | Run Playwright inside the pinned devshell (matching chromium); preconditions (backend seedable, `.env`); boot Vite with the **same frontend env as CI**; **only one Vite + run per host port**, with an uncommitted alt-port escape hatch; readiness without sleeps; confirm success; teardown; integration-project invocation. | Devshell, secrets wrapper, ports, tmux. |
| `starting-the-e2e-backend` | **Per-consumer isolated backend** (own worktree/DB/Redis via a services wrapper, a hashed port, a short slug for the Postgres socket-path length limit); boot in test mode with the async flag; seed-endpoint 404/500 semantics; the vendor-A mock; teardown (hard-kill a server that traps SIGTERM); the full integration-stack recipe. | Everything mechanical. |
| `investigating-e2e-failures` | Start from the report/trace; **localize the layer in execution order**: browser -> seed/login -> app load -> assertion; classify assertion failures (regression / stale selector / backend contract gap / seed-data / flaky / local env mismatch); `--repeat-each=5` to tell flaky from deterministic; deeper tools; integration triage. Requires systematic debugging: never fix with a timeout. | Devshell command, seed-endpoint specifics. |
| `reviewing-e2e-coverage` + agent `senior-qa-e2e-reviewer` | Report-only senior-QA audit. Intent first, then enumerate the surface from intent + backend, then a per-spec audit. **Assertion-quality rubric** [Presence-only] / [Shallow] / [Deep]: one action -> all effects; a downstream integration call is a Deep assert; the app's own API read is Shallow; round-trip is structurally blind; a fixture that can't produce the failure mode is a gap; links opened; flows exercised; full CRUD. Report shape: sources -> feature model -> per-spec audit -> matrix -> prioritized backlog (WHAT / WHERE / ASSERTION / SEED) -> gaps. Agent guardrails: no edits, no git writes, tickets read-only. | Intent source, examples, paths. |
| `qa-e2e-test-only-changes` | QA fixes stay in test code: never edit app source to make a test pass; document app fixes for devs; test-side patterns (wait for overlay hidden, fix listener registration order, user-visible completion); test-side is not shortcut-side. | Example names only. |
| `testing-a-feature-end-to-end` (+ `references/`) | A multi-stage pipeline: intake -> intent -> plan/consolidate -> backend + seed + mock -> author -> assertion depth -> run/triage -> bug hunt (fixme + finding, never green-match) -> open PR (never merge) -> quality loop with independent reviewers -> **interaction and branch completeness check** by an independent worker -> final report -> file bugs. Run-state checkpoint for respawns; operator gates table. | Tracker, intent source, author identity, backend worktrees. The reference copy said "minimal comments"; the kit copy uses the flat ban. |
| `analyzing-test-tickets` | Ticket or chat thread -> issue neighbourhood (relations, siblings, open bugs) -> intent context -> discussion search -> **Feature Analysis Report** template (overview, ACs, linked-ticket ACs, decisions, dependencies, suggested scenarios). | Tracker/chat/KB tool names, project key. |
| `sweeping-releases-for-e2e-coverage-gaps` | Freshness gate (fetch first, state the SHA) -> **enumerate from the app-source git diff between tags, not the tracker** -> trackers and intent KB for intent only -> rule out no-browser-surface items *with a recorded reason* -> rank (cost is a sort, never a filter) -> report. | Tag scheme, dual tracking. |
| `editing-backend-seeds-and-mocks` | Iron rule: no mutation in a **shared** checkout without a coordinator ack obtained first; author in your **own** backend worktree on a companion branch; never merge; never mix repos in one commit; mocks are born in the mocks repo; restart then re-prove. Its reference holds the seed and mock authoring rules (3.2, 3.5). | Repo paths, coordinator messaging. |

---

## 7. Layer 7: Hard-won rules

The generic lessons distilled from running this suite are in `03-testing-lessons.md`.

---

## 8. Generic vs specific: the coupling table

The setup mostly splits cleanly into "replicate as-is" and "re-derive". **But several conventions are only valid because of a specific backend mechanism.** Each row names the coupling, the **precondition** the new project must offer, and the **fallback** if it can't.

| # | Convention (looks generic) | Coupled to (reference mechanism) | Precondition on the new project | Fallback if absent |
|---|---|---|---|---|
| C1 | "Reach the backend ONLY through **named committed seeds** that return JSON" | cypress-on-rails `/__cypress__/command` executing `spec/cypress/app_commands/**/*.rb` in the backend process | A **test-only, non-production** backend endpoint or CLI that runs a *named, versioned* scenario script with access to domain factories and returns ids as JSON | (a) a seed library **in the test repo** calling a public/admin API (slower; can't reach internal states); (b) SQL fixture files per scenario; (c) DB snapshot restore per describe. Keep the rule "named + committed + returns what the spec needs; never ad-hoc eval". |
| C2 | "Clean before every test" (auto fixture) | `clean.rb` = DatabaseCleaner **truncation** of the whole DB | A reset op that is fast (under about 1 s) and total | Per-test **tenant isolation** (a fresh tenant per test, never truncate). That **unlocks `workers > 1`**. |
| C3 | `workers: 1` + shard-level parallelism | Follows from C2: a global truncate is unsafe with concurrent workers on one DB | none | With tenant isolation (C2 fallback) raise workers; otherwise shards need a DB each. |
| C4 | "Async end states: click, then **drain**, then poll the UI" | `seeds/transitions/run_schedulers.rb` calling `perform_now` on scheduler jobs, plus the async test flag -> `:async` adapter | A deterministic way to **drain or run** background work in test mode, exposed as a named seed/command | Poll the UI with a long `expect.poll`/`reloadUntil` against real workers; mark jobs that can't be drained deterministically as no-UI coverage. |
| C5 | Auth shortcut: API sign-in -> token planted in `localStorage` before navigation | A JWT returned in the `Authorization` header; the SPA reads `{{TOKEN_STORAGE_KEY}}` | A programmatic sign-in that yields whatever the app reads (token, cookie or session) | Cookie apps: `context.addCookies` / `storageState` from an API login. SSO-only apps: a test-env bypass endpoint, or a Playwright `setup` project doing one UI login per role and reusing `storageState`. |
| C6 | "Admin is created fresh per test" | `factory_bot` command with `super_admin: true`; needed because C2 wipes users | A seed that creates a privileged user | Fold into the per-test tenant seed. |
| C7 | `/__pw_setup` origin stub + cookie-consent pre-reject | The app's consent key `{{CONSENT_STORAGE_KEY}}` | none (the route-fulfil trick is generic) | Replace the key name; drop it if there is no consent banner. |
| C8 | "Assert every third-party touch at the **mock transaction log**" | Mockoon admin API (`/mockoon-admin/logs`, `.../state`) + backend outbound URLs overridable by env | The backend's third-party **base URLs must be configurable** in test mode; a mock server with a queryable request log and a reset API | WireMock (`/__admin/requests`), MockServer, or a tiny recording proxy. If URLs are hard-coded, this rule can't be met; raise it as a testability defect. |
| C9 | Verifiers match the **specific entity**, assert status, and poll until all expected calls arrive | One outbound task per destination, with several URL shapes | none | Generic; only the matchers are domain-specific. |
| C10 | `waitForGraphQLResponse('mutation X')` before clicks | The app uses GraphQL at one endpoint | none | REST: `page.waitForResponse(r => r.url().includes(path) && r.request().method() === ...)`. |
| C11 | Unstable floating-UI retry (`openUnstableFloatingSection`) | Headless UI portals rendered into dedicated root nodes | none | Drop it; write equivalents only when a real portal race appears. |
| C12 | Seed result types in a shared types file | Legacy location in the Cypress tree | none | Put them in `<e2e-root>/data/types/seeds.ts`. Keep the "never inline" rule. |
| C13 | Suite-own dep-clean utils; never import app barrels | Vite/ESM app + tsconfig `~` aliases + an env-config side-effect barrel | none | Keep the rule if the suite imports app code at all; otherwise forbid app imports entirely. |
| C14 | Fail-open `chromium` project + self-scoped side projects | Only matters if there are two or more projects with different stacks | none | Keep it as the default; it is free. |
| C15 | CI backend = prebuilt service container | A floating-tag test image built by the backend repo | A runnable backend artifact in test mode, ideally **pinned by digest or version** | Build the backend from source in the job, or run docker-compose of the backend repo at a pinned ref. Avoid floating tags. |
| C16 | Mock definitions from a separate repo | The mocks repo checked out per job | A single canonical home for mock environments | A `mocks/` folder in a dedicated repo, or in the backend repo; **not** vendored into the frontend. |
| C17 | Smoke gated on the `approved` label (added after 1 review) | `{{APPROVAL_WORKFLOW}}` + label conventions | Repo labels + a token able to label | Run smoke on every PR push (simpler, more minutes). |
| C18 | gh-pages report hosting + pruning | GitHub Pages enabled on `gh-pages`; a Pages deploy action | Pages enabled | Upload the merged HTML as an artifact only. |
| C19 | Runtime-balanced shards | `{{TIMINGS_FILE}}` + reporter + splitter + refresh PR | none | Native `--shard=i/N`. **Note:** the full machinery runs as round-robin in the reference (4.3). |
| C20 | Istanbul coverage from e2e | A coverage env flag + Vite Istanbul plugin + `window.__coverage__` + nyc + CI warm-up (`global-setup.ts`) | Instrumentable build | Omit initially. |
| C21 | Admin-panel (server-rendered) POs on the backend origin | A server-rendered admin + a passwordless sign-in route gated on `{{REVIEW_APP_FLAG}}` | Only if the product has a server-rendered admin | Treat it as a second origin with its own login helper. |
| C22 | Intent oracle for coverage reviews | A product-knowledge wiki + `{{INTENT_SKILL}}` | Some written product intent (PRD, specs, ACs) | Tickets' ACs plus designs; flag where intent is missing. |
| C23 | Tracker / bug filing | One tracker, with some work dual-tracked in a second | A tracker | Re-point `{{BUG_FILING_SKILL}}`. |
| C24 | Local runtime | NixOS devshell with pinned chromium; a services wrapper; spore per-slug ports, tmux, `{{SECRETS_WRAPPER}}` | none | Re-derive per host. Keep the **one-backend-per-consumer** and **CI-equal env** rules. |
| C25 | Multi-service harness project (`integration`) | Integration gateway + message queue + S3 emulator + vendor-B mock | Only if the product has an async integration pipeline worth e2e-ing | Add later as a separate self-scoped project with its own label-gated workflow. |

**Couplings that are easy to miss:**
- **C1 -> the no-console/eval rule.** The ban on `eval` reads is enforceable only because a richer, sanctioned channel exists (named seeds, plus the `get_item_status` transition seed as a status oracle for *waiting*, not asserting). On a backend without a seed endpoint, the new project must still provide a sanctioned *wait* channel, or "UI is the only oracle" becomes "poll the UI forever".
- **C2 -> C3 -> CI shape.** The larger full-suite shard matrix exists partly because truncation forces `workers: 1` (1.1, [HYPOTHESIS]). Choosing tenant isolation on the new project changes the CI math entirely.
- **C8 depends on a backend testability property** (env-overridable outbound URLs). It is not a test-side choice.

---

## 9. CI: minimal viable vs full

**Minimal viable (recommended as phase 1 on the new project):**
1. One reusable workflow, two callers:
   - PR (every push, or after approval): `--grep @smoke --shard=i/2..3`;
   - PR into the release branch: full run with `--shard=i/N`.
2. Services: DB + the backend in test mode at a **pinned version**. Steps: start the mock server(s) from the canonical mocks home, then `wait-on` health; install; `playwright install chromium`; run `playwright test --project=chromium`.
3. Config: `retries: 2` on CI, `trace: retain-on-failure`, `reporter: blob` on CI, `forbidOnly` on CI, `maxFailures` off (or high) until the suite is stable.
4. A `merge-reports` job: `if: always()` -> `npx playwright merge-reports --reporter html` -> upload the artifact (30-day retention).
5. A `skip tests` label escape hatch.
6. Lint and type-check the test folder in the normal CI. **The reference lacks this** (1.4), and it is cheap.

**Full (add only when each pays for itself):**

| Add-on | Value | Cost / risk | What the reference shows |
|---|---|---|---|
| `approved`-label gating | Saves minutes on unreviewed PRs | "Skipped is not green" confusion | Skipped smoke checks were mistaken for passes. |
| Runtime-balanced splitter + timings reporter + auto-refresh PR | Balanced shards on a large suite | 3 scripts + 1 workflow + a refresh PR someone must merge | The timings file is **empty**, so the splitter is round-robin; no benefit was realised. |
| Diff-scoped smoke tags | Run feature tags touched by the diff | A mapping file to maintain | The mapping is **empty**, so the grep is always `@smoke`. |
| gh-pages publishing + weekly prune | Permanent per-attempt report links | Pages config, a prune job with truncation guards | Useful for forensics: a published report outlives a re-run that replaces the run artifacts. |
| E2e coverage (Istanbul + nyc + PR comment + CI warm-up) | Coverage signal | Instrumented cold compile -> needed a `global-setup` warm-up | `global-setup.ts` exists only for this. |
| Label-gated multi-service project workflow | Integration e2e | Heaviest job; near runner disk limits | Size the runner for it: a full multi-service stack can exhaust a hosted runner's disk. |
| Post-deploy prod smoke (own config, read-only) | Production canary | Canary data in prod | `{{PROD_SMOKE_CONFIG}}`, `prodSmoke/CLAUDE.md`. |
| Cross-repo release gate (dispatch + poll) | Couples a sibling app's suite to release | Cross-repo token, polling | The sibling report-app gate. |

**Recommendation:** ship minimal CI with native `--shard`, a digest-pinned backend and test-folder lint. Add gh-pages publishing next, because forensics value is high and cost is low. Add the splitter only once per-shard wall time is long and native sharding is measurably skewed **[judgement, not measured]**.

---

## 10. Known unknowns about the reference

These were not determined when the reference was studied. Treat the related statements above as softer than the rest.

1. **No CI history was read.** Whether timings-refresh PRs were ever opened, why the timings file is still empty, real shard durations, flake rates, and how often smoke is skipped are all unmeasured.
2. **The reason for `workers: 1`** is not written anywhere. Truncation-safety is a **[HYPOTHESIS]**.
3. **Whether the `{{LEGACY_RUNNER_FULL_LABEL}}` label triggering a full Playwright run is intended** or a leftover. The suite doc disagrees with the workflow.
4. **How the CI backend image is built** was not read.
5. **The mocks repo itself** was not read; statements about it come from the seeds-and-mocks guide and the workflow files.
6. **The integration workflow and its folder doc** were skimmed, not read line by line.
7. **Branch protection / required checks** (which Playwright jobs block merge) were not inspected.
8. **Backend version pairing:** the backend was read at its integration-branch tip on the same day, not at a commit known to pair with the frontend commit studied.
9. **Comment-rule drift:** support files and POs carry comments that violate the no-comment rule; the rule was not dated against them.
10. **Only one spec was walked end to end** (2.3). The others were counted and grepped (tags, seed calls).
11. **The suite `.env`** is gitignored with no example; the variable list in 4.2 was reconstructed from code and may be incomplete.
