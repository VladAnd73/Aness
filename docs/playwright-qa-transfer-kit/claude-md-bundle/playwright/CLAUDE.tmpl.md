# {{PROJECT_NAME}} - Playwright e2e Suite (root context)

This is the **suite index and root context** for the Playwright e2e tests. Every folder under
`playwright/` has its own `CLAUDE.md` describing what it holds, how to use it, and the conventions
that apply there. Start here, then follow the cross-links down into the folder you're working in.

## Role Context
Primary focus for this repo is:
- **Playwright e2e tests** (active development - smoke & regression suites)
- **Cypress tests** (legacy - maintenance only; do NOT scaffold new Cypress specs)
- **GitHub Actions CI** (workflows in `.github/workflows/`)
- Occasionally adding `data-testid` attributes to frontend components

## Folder map (each has its own CLAUDE.md)

```
playwright/
|-- support/        - infra: the test fixture, seeding server, login/tenant flows, mock + integration helpers
|-- pageObjects/    - Page Object Model
|   |-- BasePage.ts - base class ALL page objects extend
|   |-- components/ - reusable UI: drawers/, modals/, sections/ (tables), headers/, AppSidebar
|   `-- pages/      - one class per route/screen (Item, Record, Group, AdminPanel, ...)
|-- data/           - client-side test data
|   |-- factory/    - faker builders that fill FORMS (not backend seeds)
|   |-- models/     - TypeScript types + preset constant arrays
|   `-- fixtures/   - static GraphQL-mock payloads + a binary media upload asset
|-- tests/
|   |-- main/        - the main-area specs (default `chromium` project), by domain
|   |                  (item, record, adminPanel, ...; `frozenDomain/` is skipped - see below)
|   `-- integration/ - the integration-as-user harness specs (`--project=integration`), full stack
|-- prodSmoke/      - the post-deploy PRODUCTION smoke: own config, read-only, never in the regular suite
`-- tsconfig.json   - path aliases for the suite (see the `~` table below)
```

## A frozen domain is out of scope - its specs are skipped, not deleted

<Optional section; keep it if a product area is frozen.> Every `test.describe` under
`tests/main/frozenDomain/**` is `test.describe.skip`. The files stay in place as a record; they collect
and report as skipped, and CI runs none of them. **Do not un-skip a describe and do not add a spec under
`frozenDomain/`** - put coverage in the matching `tests/main/<domain>/` folder instead. `.skip` is the
chosen mechanism: the CI workflow, the shard config and `playwright.config.ts` are deliberately untouched.

## Two Playwright projects (`playwright.config.ts`)

- **`chromium`** (default) - `testDir: ./playwright/tests` + `testIgnore: '**/tests/integration/**'`,
  i.e. everything except the integration folder. It is deliberately **fail-OPEN: a spec in a new folder
  runs by default.** Do NOT re-scope it to a specific folder - a spec outside that folder would then be
  collected by no project at all, with no warning and a green exit 0. This is what a plain
  `yarn playwright test` and the always-on CI smoke/regression suite run.
- **`integration`** - `testDir: ./playwright/tests/integration`, selected with `--project=integration`.
  Drives the REAL {{INTEGRATION_PIPELINE}} (backend -> message queue -> integration gateway ->
  {{VENDOR_B}} mock) and needs a full out-of-band stack. It is always registered but self-scopes via its
  `testDir`, so a `--project=chromium` run never sweeps it in. See
  [`tests/integration/CLAUDE.md`](tests/integration/CLAUDE.md).

## The production smoke (`playwright/prodSmoke/`) - its own config

`{{PROD_SMOKE_CONFIG}}` runs `playwright/prodSmoke/` **against production**, read-only, with no
backend, no Vite, no `globalSetup` and no suite fixtures. It sits outside `playwright/tests`, so neither
project above nor the shard splitter ever collects it. CI runs it via `{{PROD_SMOKE_WORKFLOW}}` after a
backend production deploy. See [`prodSmoke/CLAUDE.md`](prodSmoke/CLAUDE.md).

## Running the environment

Backend ({{BACKEND_REPO}}, test env). On {{HOST_OS}}, bring the per-slug backend up via the
backend-boot skill and run Playwright via the e2e-runner skill (chromium comes from {{DEVSHELL}}).
The canonical raw commands:

```bash
# Backend (per-slug isolated stack)
{{ASYNC_TEST_FLAG}}=true {{BACKEND_BOOT_COMMAND}}
# Frontend (Vite on {{FRONTEND_PORT}}). The config has a webServer with reuseExistingServer locally, so it
# reuses an already-running Vite; boot Vite yourself if auto-start is unreliable on your host.
{{BACKEND_URL_ENV}}={{BACKEND_URL}} yarn start
```

```bash
yarn playwright test                                                        # main area (chromium)
yarn playwright test playwright/tests/main/item/createItem.spec.ts          # single file
yarn playwright test --grep @smoke                                          # smoke suite
yarn playwright test --headed --ui playwright/tests/main/path/to/test.spec.ts # debug UI mode
yarn playwright test --project=integration                                  # integration harness (own stack)
```

## The `~` import alias (READ THIS - it is the #1 trap)

Resolution is defined in `tsconfig.paths.json`:

| Import | Resolves to | Use for |
|--------|-------------|---------|
| `~/playwright/*` | repo-root `playwright/*` | fixtures, page objects, support, data (this suite) |
| `~/cypress/*` | repo-root `cypress/*` | **shared type defs** (`~/cypress/common/types/seedTypes`, `memberTypes`, `subunitTypes`, ...) |
| `~/*` | `src/*` | the **application** source |

> **NEVER import `~/utils` (or other `~/...` app barrels) into a page object or spec.** `~/utils` is the
> app's `src/utils` barrel; it re-exports the app's env config and blows up under Playwright's loader with
> `"exports is not defined in ES module scope"` - the whole PO import chain fails and Playwright reports
> **"No tests found."** If you need a helper, import the suite's own leaf `~/playwright/support/utils`,
> or a dep-clean app leaf file (not the barrel), or inline the tiny formatter. Shared types come from
> `~/cypress/common/types/*`, never from an app barrel. The integration harness's `MockClient` /
> `IntegrationBackend` base classes and the shared `support/backendApi` request primitives are
> deliberately kept dep-clean for the same reason, so the `integration` project can load them.

## How the suite is wired (verified against `support/fixtures.ts` + `playwright.config.ts`)

- **`testDir: ./playwright/tests`** with per-project scoping (see above), `baseURL: {{FRONTEND_URL}}`,
  `fullyParallel: true` but `workers: 1`, `retries: 2` on CI / `0` locally, `reporter: blob` on CI /
  `html` locally, `timeout: 60s` CI / `40s` local, `trace: retain-on-failure`,
  `globalSetup: ./playwright/support/global-setup.ts` (warms the app shell on CI only).
- **Import `test` from the suite fixtures, never from `@playwright/test`:**
  ```typescript
  import { test } from '~/playwright/support/fixtures';
  ```
  (every spec - main area AND integration)
- An **`auto` fixture `forEachTest`** runs before EVERY test: `new Server().seed('clean')`, then purges
  the {{VENDOR_A}} and {{VENDOR_B}} mock transaction logs (vendor B also resets state - no-op when that
  mock's base URL is unset), then `new UserFlows(page).loginWith()` (creates an admin + logs in via the
  localStorage token). You do not seed `clean`, purge mocks, or log in yourself.
- The fixture object also injects, by name: `userFlows`, `server`, `vendorAMock`, and page-object
  fixtures <list every registered page-object fixture name, e.g. `homePage`, `appSidebar`, `membersPage`,
  `recordsPage`, `recordPage`, `itemsPage`, `itemPage`, `contactsPage`, `editItemPage`, `dashboardPage`,
  `loginPage`, `orderManagementPage`, `failedItemsListPage`, `adminItemShowPage`>. **Destructure the
  fixture you need - don't `new` up a second copy.** Page objects NOT in this list (most drawers/modals,
  the singular detail pages, other admin show pages) are instantiated inline with `new X(page)`.
- The same fixture also carries the **integration harness** fixtures `integrationOrders`,
  `integrationImports` (value-scoped) and `vendorBMock` (worker-scoped); only the `integration`-project
  specs destructure them, and Playwright's lazy fixtures mean the main-area specs pay nothing for them.

## Standard spec shape

```typescript
import { test } from '~/playwright/support/fixtures';

let seeded: SomeSeedType;

test.describe('Feature | Action', () => {
  test.beforeEach(async ({ page, server, userFlows, pageFixture }) => {
    seeded = await server.seed('seeds/main/something');
    await userFlows.changeTenantTo(seeded.tenant);
    await page.goto(pageFixture.url);
  });

  test('does the thing @smoke', async ({ pageFixture }) => {
    await pageFixture.validateVisible();
  });
});
```

The seed type comes from `~/cypress/common/types/seedTypes` and is never declared inline; call
`changeTenantTo` only if the seed returns a tenant; the test body drives the real UI via page-object
methods and asserts the END STATE the user sees.

## How seeds work

- Seeds are **scripts in the backend repo** at `{{SEED_DIR}}`, run via HTTP POST to `{{SEED_ENDPOINT}}`.
  The suite reaches the backend ONLY through these named committed seeds - never via `eval` / a
  framework console runner / direct ORM reads.
- `server.seed('seeds/...')` returns the seed's JSON (uuids, tokens, tenant, entities). If a test
  needs a value, the seed must RETURN it; editing a seed goes through the seed-and-mock editing skill
  (cross-repo).
- Item lifecycle helpers live on `Server`: `takeItemToStatus`, `waitForItemStatus`,
  `waitForItemPublished`, and `runScheduler()` (drives async jobs via the canonical
  `seeds/transitions/run_schedulers` - click the real action, then drain with {{DRAIN_COMMAND}}, then
  poll the UI).

## Suite-wide conventions (enforced everywhere - echoed by the `new-test` skill)

1. **Assert the end state through the UI the user sees** - a rendered badge/row/count/status via a page
   object. Never read the app's own GraphQL/REST response as a UI stand-in, and never read state via
   the backend console.
2. **One action -> many effects; assert them ALL.** Primary state change is mandatory; a banner/toast is
   a *secondary* check, never sufficient alone.
3. **Prove every third-party touch at its mock transaction log.** {{VENDOR_A}} via `support/vendorAMock.ts`
   (`purgeLogs()` -> action -> `verifyVendorReceived*`); {{VENDOR_B}} via `support/vendorBMock.ts`
   (`expectRecordResolved`/`expectOrderStatusPosted`/...). If a flow should hit a vendor but nothing
   asserts it, that's a coverage hole - flag it.
4. **No locators in specs.** All selectors live in page objects; specs only call PO methods and read as
   a plain-English scenario. Prefer `getByRole`/`getByTestId`.
5. **Reuse before you write.** Search for an existing PO/fixture/helper/seed and use (or generalize) it;
   never fork a parallel copy.
6. **No standalone "page loads" test.** Assert list/page content inline at the top of a functional test
   that loads the page anyway (e.g. a `validateXList(items)` helper). Merge tests that share seed+setup.
7. **No comments in test code. None.** Not in a spec, a page object or a support file, and with no
   exception for user-behaviour notes or ticket links. **Never explain `src/` in the tests**: not component or file names, props, render or query
   timing, test-id schemes, routing rules. The tests reflect the app through the routes they drive, not
   through prose about the implementation. Provenance (tickets, commits, backend file references,
   root-cause analysis) goes in the PR or the ticket; conventions go in a folder `CLAUDE.md`; a
   workaround keeps its code and loses its explanation. Don't relocate the prose into test names,
   `test.step` labels or variable names.
8. `@smoke` = runs on every PR (smoke suite); untagged = full regression only.
9. **What to assert** is governed by [`.cursor/rules/testing.mdc`](../.cursor/rules/testing.mdc), applied
   at the decision points: [`tests/CLAUDE.md`](tests/CLAUDE.md) and [`pageObjects/CLAUDE.md`](pageObjects/CLAUDE.md).

## CI behavior
- PRs with the `approved` label run `@smoke` only ({{SMOKE_SHARDS}} shards, native `--shard=X/Y`).
- Label `{{FULL_REGRESSION_LABEL}}` -> all tests ({{FULL_SHARDS}} shards, runtime-balanced via `{{TIMINGS_FILE}}`);
  `{{LEGACY_RUNNER_FULL_LABEL}}` -> all Cypress; `skip tests` -> skip.
- PR into the production branch or `{{RELEASE_BRANCH_PATTERN}}` -> all tests, runtime-balanced, and
  collects fresh timings.
- CI does 2 retries (0 locally); reports are blob -> merged to HTML on GitHub Pages, HTML locally.
- Every CI Playwright invocation passes `--project=chromium`, so the `integration` project never rides
  along. It runs via its own `{{INTEGRATION_WORKFLOW}}` (needs the full stack) - see
  [`tests/integration/CLAUDE.md`](tests/integration/CLAUDE.md).

### Sharding & timings
- **Full runs** ({{FULL_SHARDS}} shards) partition by runtime using `{{TIMINGS_FILE}}` (repo root).
  `{{SHARD_SPLITTER_SCRIPT}}` reads per-file weights and greedy-LPT bin-packs into `--total` shards.
- **Smoke runs** ({{SMOKE_SHARDS}} shards) keep native `--shard=X/Y` (already well balanced; no timings needed).
- Weights auto-refresh: the full-regression workflow sets `collectTimings: true`; each shard writes
  a per-shard partial of `{{TIMINGS_FILE}}` via `SpecTimingReporter`; `{{TIMINGS_REFRESH_WORKFLOW}}` fires on
  that workflow's success, merges the shard artifacts, and opens a draft PR against
  `{{DEFAULT_INTEGRATION_BRANCH}}` with the updated file.
- Manual refresh: run the timings-refresh workflow via `workflow_dispatch`, passing the full-regression
  `runId` to source timings from.
- The splitter enumerates specs by **walking `playwright/tests` on disk** - the timings file only supplies
  weights, so a moved or renamed spec is never dropped, it just falls back to the median weight of known
  files (empty timings -> equal weights). It walks `tests/integration` too; those paths land in a shard's
  file list and contribute no tests to the `--project=chromium` run.

## Cross-links
- Infra: [`support/CLAUDE.md`](support/CLAUDE.md)
- Page objects: [`pageObjects/CLAUDE.md`](pageObjects/CLAUDE.md)
- Test data: [`data/CLAUDE.md`](data/CLAUDE.md)
- Production smoke: [`prodSmoke/CLAUDE.md`](prodSmoke/CLAUDE.md)
- Specs: [`tests/CLAUDE.md`](tests/CLAUDE.md) -> main area [`tests/main/CLAUDE.md`](tests/main/CLAUDE.md) and integration harness [`tests/integration/CLAUDE.md`](tests/integration/CLAUDE.md)

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
