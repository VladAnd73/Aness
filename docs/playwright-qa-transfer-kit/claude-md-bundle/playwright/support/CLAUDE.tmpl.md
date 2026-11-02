# playwright/support - Test infrastructure

The plumbing every spec sits on: the custom `test` fixture, backend seeding, auth/tenant flows, and
the mock / integration transaction-log helpers. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **fixtures.ts** - the heart of the suite. Exports `test` (an extend of `@playwright/test`). Defines
  the `auto` fixture `forEachTest` that runs before EVERY test: `Server().seed('clean')` ->
  `MockClient({{VENDOR_A_MOCK_URL_ENV}}).purgeLogs()` -> `MockClient({{VENDOR_B_MOCK_URL_ENV}})`
  `.purgeState()` + `.purgeLogs()` -> `UserFlows(page).loginWith()`. The per-test mock resets no-op when
  a mock's base URL is unset (i.e. that mock isn't booted for this run), so the same fixture is safe on
  every run. On teardown it collects Istanbul frontend coverage when `VITE_COVERAGE` is set. Registers
  all page-object / utility fixtures by name (`userFlows`, `server`, `vendorAMock` = `new VendorAMock(
  {{VENDOR_A_MOCK_URL_ENV}})`, `itemsPage`, `recordsPage`, ... - the full list is in the root
  [`../CLAUDE.md`](../CLAUDE.md)). It also carries the **integration harness fixtures** -
  `integrationOrders` / `integrationImports` (value-scoped, disposed after each test) and `vendorBMock`
  (worker-scoped) - reading the `BACKEND_URL` / `{{VENDOR_B_MOCK_URL_ENV}}` / vendor-B webhook Basic-auth
  env vars (local defaults `{{BACKEND_PORT}}` / `{{VENDOR_B_MOCK_PORT}}` / the dev-only credential pair
  the backend's test env also defaults to). Playwright fixtures are lazy, so specs that don't
  destructure them pay nothing. **Every spec - main area and integration alike - imports `test` from
  here.** A second `auto` fixture, `resetIntegrationGateway`, is the integration analogue of
  `forEachTest`'s backend clean: gated on `testInfo.project.name === INTEGRATION_PROJECT` (a no-op for
  every other project), it calls `IntegrationGateway().reset()` before each integration test so the
  gateway's accumulating tables start each test empty (tenants preserved).
- **projects.ts** - the Playwright **project names** as exported consts (`INTEGRATION_PROJECT`). Imported
  by both `playwright.config.ts` (which declares the projects) and any fixture that scopes itself to one,
  so the name exists once. **Gate a project-scoped fixture on this const, never on a string literal** - a
  literal that drifts from the config makes the fixture silently stop running, and a fixture that
  silently stops running takes its isolation with it while the suite still looks green.
- **backendApi.ts** - the **dep-clean low-level e2e-backend request primitives** shared by `Server`,
  `UserFlows`, and `IntegrationBackend` (imports ONLY `@playwright/test`, so it never taints the
  integration project's load). `postCommand(ctx, backendUrl, name, options?)` -> POSTs a seed command and
  returns the raw JSON, throwing an error that **names the command** on a non-2xx/3xx (the command path is
  identical for every seed in the repo, so a URL-only message makes every spec's seed failure look alike -
  keep the name in any error you add on this path); `unwrapSeedResult(body)` -> the seed endpoint's
  array/JSON-string unwrap; `signInForBearer(ctx, backendUrl, email, password)` -> POST `/users/sign_in`
  and return the bearer token; `graphqlQuery(ctx, backendUrl, query, variables, bearer, tenantUuid)` ->
  POST `/api/graphql` tenant-scoped and return `data`, **throwing on an `errors` payload** (GraphQL errors
  come back HTTP 200, so a status check alone reads a failed query as empty data).
  `IntegrationBackend.graphql` and `Server`'s readiness polls both delegate here - don't hand-roll a
  second `/api/graphql` POST. The caller owns the `APIRequestContext` and `backendUrl`, so the
  reused-vs-fresh context and injected-vs-module-env differences stay with each caller.
- **server.ts** - `Server` class. Talks to the backend via the seed endpoint `{{SEED_ENDPOINT}}` on `BACKEND_URL`
  (through `backendApi.postCommand`; `seed` unwraps with `backendApi.unwrapSeedResult`).
  Key methods: `seed(name, options?)` -> runs a backend seed and returns its parsed JSON;
  `runScheduler()` -> drains async jobs via `seeds/transitions/run_schedulers` ({{DRAIN_COMMAND}});
  `takeItemToStatus(uuid, status)`, `waitForItemStatus(uuid, status?, opts?)`,
  `waitForItemPublished(uuid, opts?)` -> item lifecycle (60s local / 120s CI polling);
  `waitForBundleItemAttachments({ itemUuid, tenantUuid, minimum?, timeout? })` -> polls the app's own
  attachments query until the bundle item has at least `minimum` attachment groups and **none of them is
  still processing**, and returns how many. **Always gate `completeBundleItemSetup` on it.** Ordering a
  bundle enqueues attachment processing asynchronously, so the create call returns before any group exists,
  and the setup seed iterates what it finds - on an empty collection it links nothing, raises nothing,
  and the item goes on to publish an empty payload. Waiting for existence alone is not enough either: the
  seed also refreshes derived children, which pairs each target with the attachments already in the group,
  so a group that exists but has not finished processing yields an item with targets and no attachments. Nothing
  downstream fails at the real cause, so the race surfaces much later as a stalled phase or a
  missing-attachments assertion; `completeBundleItemSetup(uuid)` -> `seeds/transitions/complete_bundle_item_setup`,
  linking a target onto every attachment group a bundle item just produced - the service that normally attaches
  targets is disabled in the test env, so a bundle item never gets one on its own; needed before a real
  publish of a bundle item carries any assets;
  `createAdmin()` (factory-built admin used by login), `insertCassette`/`ejectCassette` (VCR control).
- **userFlows.ts** - `UserFlows` class: auth + tenant context. `loginWith()` (the auto-login default),
  `loginWithEmailAndPassword()` (public; signs in via `backendApi.signInForBearer` then stores the token
  in `localStorage` under `{{TOKEN_STORAGE_KEY}}` - the same sign-in primitive `IntegrationBackend.signIn`
  returns), `loginTenantUserWithRole(tenantUuid, role)`, `createUserWithRole(role, tenantUuid?, email?)`,
  `assignUserToTenants(email, uuids)`, `changeTenantTo({ uuid })` (sets the active tenant in localStorage
  under `{{TENANT_STORAGE_KEY}}` - takes just a uuid), `signInAsAdmin()` (admin-panel review sign-in via
  the backend's review sign-in route - needs `{{REVIEW_APP_FLAG}}` on the backend),
  `clearSession({ cookies? })`. **Pass `cookies: true` in any spec that then signs in with credentials and
  expects the credentials to be what decides the outcome** - a surviving server session cookie can carry a
  sign-in through, so a "these credentials no longer work" assertion would pass for the wrong reason.
  `ensureOriginContext()` also stubs `/__pw_setup` and rejects the cookie-consent banner
  (`{{CONSENT_STORAGE_KEY}}`) so it never occludes the UI.
- **mockClient.ts** - `MockClient`, the **dep-clean base** for talking to any {{MOCK_TOOL}} server,
  addressed by its base URL (`new MockClient(baseUrl?)`). `purgeLogs()`, `purgeState()` (data buckets +
  request counters), `getLogs()` (paginates the admin log), `waitForLogs(predicate, opts?)`
  (`opts.minMatches` waits for N matches instead of returning on the first, so an exact-count assertion
  can't race the later calls), `dispose()` (releases the APIRequestContext - fixtures call it in teardown).
  The admin path (`MOCK_ADMIN_PATH`, Mockoon's `/mockoon-admin`) is a code constant (same on every
  server); only the base URL varies. `baseUrl` undefined -> purges no-op and log reads throw. Kept free of
  app-source imports so the integration project loads it without the ESM barrel trap. Exports
  `MockLogEntry`.
- **vendorAMock.ts** - `VendorAMock extends MockClient`: the **{{VENDOR_A}}**-mock assertions, addressed
  by `{{VENDOR_A_MOCK_URL_ENV}}`. `verifyVendorReceivedArchiveUpdate` / `PauseUpdate` / `ActiveUpdate`
  (match a status POST for a `targetUuid` on the destination paths) and
  `verifyVendorReceivedRepublish(itemName, destinationTypes)` (the republish task POST, matched by item
  name since seeds can share a target), plus its complement `verifyVendorReceivedNoRepublish()` for a
  rejected republish (no wait - asserts the log holds no task at all, so purge immediately before the
  action), and `verifyVendorReceivedVendorContracts({ itemName, contractIds, targetCount, settleMs? })`, which
  polls for ONE task of the relevant destination naming the item (so purge before each republish),
  requires a 2xx, the expected number of targets, and the same contract ids (order-insensitive, a missing key
  fails) on every one of them - then waits `settleMs` (default 2s) and re-counts outside the poll, so a
  second task arriving after the first fails instead of being missed. All the task readers go through one
  private `taskEntriesFor(itemName, taskKeys)` (keys from `DESTINATION_TO_TASK_KEY`); add a new task
  reader on top of it rather than re-parsing the log. Every verifier asserts the response **status** so a
  404 fall-through fails instead of greening on a mere hit.
  **A republish is ONE task POST PER DESTINATION, and each destination names its entry differently** -
  bare `<item name>`, `<item name> - <Destination>`, and `<uuid> - <item name>` are all in use. So
  `verifyVendorReceivedRepublish` matches a name three ways (equal / prefix / suffix) and **polls until
  every expected task key has arrived** rather than asserting on the first task logged. Do not "simplify"
  either half: a single-shape matcher silently never matches one destination, and a one-shot read races
  the destinations that post later - both of which fail on the destination, not on the item, so they read
  like a product bug. Add the shape to `taskEntryNamesItem` when a new destination appears.
  `verifyVendorReceivedDistinctAttachments({ itemName, expectedAttachmentCount })` (also a task POST, so it
  reuses `taskEntryNamesItem` and the same poll-until-arrived pattern) asserts a bundle task's attachment
  arrays carry exactly N entries referencing N distinct files (a bug that sends one file N times still
  fails on the deduplicated count).
  `verifyVendorReceivedPost({ destination, actorId, message })` - the post paths, matched by actor id and
  requiring a 2xx (a request that fired but failed doesn't count as "sent").
  `verifyVendorReceivedNoPost({ destination, actorId })` is the negative counterpart - it matches on the
  same request shape but regardless of response status, so a failed attempt still counts as "the app
  tried something" rather than silently passing. Actor-id matching (not path alone) also isolates against
  a retried request from an earlier test landing in a later test's log window.
- **vendorBMock.ts** - `VendorBMock extends MockClient`: the **{{VENDOR_B}}** outbound-contract
  assertions for the integration harness, addressed by `{{VENDOR_B_MOCK_URL_ENV}}`.
  `expectRecordResolved(recordId, { times? })`, `expectRecordPublicationsResolved(recordId, { times? })`,
  `expectOrderStatusPosted(orderId)` (204 callback), `expectRecordAttachmentsFetched(recordId, attachmentCount)`
  (import proof: list + each file). Named after the vendor API calls so specs read intent; all four
  delegate to one private `expectCalls(method, path, { status, times?, timeout? })`. Exact counts
  back-stop a skipped per-test purge - pass `times` when the flow legitimately resolves the same record
  once per leg (sync, then order).
- **integrationBackend.ts** - `IntegrationBackend`, the **dep-clean base backend driver** for the
  integration harness. Owns the reused `APIRequestContext` and the shared paths: `seed(name, options?)`
  and `signIn(email, password)` -> bearer token both delegate to the `backendApi` primitives (passing the
  reused context + constructor `backendUrl`); `seedRequiring(name, keys, options?)` wraps `seed` and
  throws naming any returned key that is blank, so a half-run seed fails at the seed instead of far
  downstream; `graphql(query, vars, bearer, tenantUuid)` delegates to `backendApi.graphqlQuery` with this
  class's reused context and `backendUrl`. `backendUrl` comes via the constructor (no module-load
  `process.env` read). `dispose()`. `IntegrationImports` and `IntegrationOrders` extend it.
- **integrationImports.ts** - `IntegrationImports extends IntegrationBackend`. `seedVendorImport()` runs
  `seeds/integration/vendor_b_import` (fires the real sync job -> fetch RPC -> gateway fetches the record
  + attachments from the vendor B mock -> uploads to LocalStack S3 -> subscriber attaches assets);
  `waitForImportedRecord(...)` polls the backend's own GraphQL until the imported record has its
  media-attached assets (proof the async chain landed).
- **integrationOrders.ts** - `IntegrationOrders extends IntegrationBackend` (ctor also takes the vendor B
  Basic auth). `seedVendorOrder({ state?, record_kind? })` runs `seeds/integration/vendor_b_order` (fires
  the create-tenant RPC); `seedVendorExistingRecordLink(variant)` runs
  `seeds/integration/vendor_b_existing_record_link` - the same order preconditions plus an existing record
  synced from one of several mock variants (the seed syncs nothing, the order does), returning the
  configured link template so a spec derives the expected destination instead of hard-coding it, plus the
  external id of the upstream entry the gateway should pick for that variant, so the spec asserting WHICH
  entry won reads it from the seed rather than duplicating the mock's fixture value;
  `awaitWriteBack(recordId)` runs `seeds/integration/await_write_back`, the readiness gate for a write-back
  leg (needed before snapshotting an order, and the only option because that leg's marker has no UI or
  GraphQL surface); `fireVendorOrder(orderId, seed)` drives the inbound order webhook with Basic auth and
  returns the HTTP status; `resolveActivatedOrder(orderId, seed)` finds the created instance via the
  authenticated bundle-instances query and mints the page preview token via the same mutation
  the main-area menu calls.
- **integrationGateway.ts** - `IntegrationGateway`, a dep-clean helper (`child_process` only) that resets
  the dockerized integration-gateway DBs by invoking the gateway repo's own reset by name: `docker exec
  <gateway-container> bash {{GATEWAY_RESET_SCRIPT}}`. The reset itself (a pure-psql `TRUNCATE ...
  RESTART IDENTITY CASCADE` of both databases minus a preserve-list, no framework boot, no reseed) lives
  in the gateway repo - this side carries no SQL or schema knowledge, so gateway schema drift is handled
  there. The container name defaults to one derived from the compose project, overridable via
  `{{GATEWAY_CONTAINER_ENV}}` (or `{{GATEWAY_COMPOSE_PROJECT_ENV}}`). Set `{{GATEWAY_REPO_ENV}}` to
  a gateway checkout instead when the gateway runs natively (no container to exec into) - the same script
  runs there directly, but the container's environment then has to come from the run: export
  the test-env flag, `{{GATEWAY_DB_URL_ENV}}`, `{{GATEWAY_SECONDARY_DB_URL_ENV}}` and put `psql` on `PATH`, or every
  integration test fails in the reset fixture before its first assertion. (The gateway's local dotenv file
  is not shell-safe - its URLs contain `&`, so read the values out rather than `source`-ing the file.)
  Driven by the `resetIntegrationGateway` auto fixture; the gateway has no per-test truncation of its own,
  so this keeps each integration test off the previous one's records/orders. It is gated on the shared
  `INTEGRATION_PROJECT` const from `projects.ts`, not a string literal - see that entry.
- **integrationStatusFromSeed.ts** - `expectedSyncDotVariant(target)` -> derives the expected sync-status
  dot (`connected` / `selectiveSync` / `disconnected`) from a target's seed data, so sync-status specs
  assert the variant the seed implies instead of hard-coding it.
- **utils.ts** - the suite's OWN dep-clean util leaf (import this, NOT `~/utils`). Array pickers,
  date/price formatters (`formatPrice`, `browserDateFormat`, `formatDateLikeBrowser`,
  `formatItemDurationRangeLikeBrowser`), `phoneDigitsPattern`, `getPhaseDisplayName`
  (`'pending'`->`'Draft'`), the phase-transition oracles `phaseAfterUnarchive(item)` /
  `phaseAfterResume(item)` (derive the destination phase from the item's own publish timestamps and start
  date instead of hard-coding one), media-upload path helpers (`generateMediaFilesForUpload`,
  `generateVideoFileForUpload`), and `commonPrefix(values)` (derive a search term several seeded values
  share instead of hard-coding it).
  **For a value the app renders formatted, derive the expectation instead of hard-coding it**:
  `formatDateLikeBrowser(page, iso)` formats in the page, so the browser's own locale decides (the range
  helper is built on it), and `phoneDigitsPattern(number)` matches the digits in order whatever separators
  the app inserts. A literal `'01/09/2026'` or `'+1 555 010 0199'` is an assertion about the runner's
  locale, not about the app.
- **global-setup.ts** - Playwright `globalSetup`: on CI only, warms the app shell (`/`, `/orders`) so the
  first shard doesn't time out on cold instrumented compile. **No mock-log purge here** - the
  `forEachTest` auto fixture resets the mocks before every test (and retry), so a run-start purge is
  redundant.

## How to use it
- **In a spec:** `import { test } from '~/playwright/support/fixtures';` then destructure the fixtures you
  need - `async ({ page, server, vendorAMock, itemsPage }) => ...`. Never re-instantiate `Server`,
  `UserFlows`, or `VendorAMock`; use the fixture instances. (The `integration` project imports the same
  `test` from here and destructures `integrationOrders`, `integrationImports`, `vendorBMock`.)
- **Seed + navigate** in `beforeEach`: `const s = await server.seed('seeds/...'); await userFlows.changeTenantTo(s.tenant); await page.goto(pageFixture.url);`
- **Async end states:** click the real UI action -> `await server.runScheduler()` -> poll the UI (a PO
  `waitFor...` method). Don't trust the async job adapter to drain on its own; don't write a per-test
  shim.
- **{{VENDOR_A}} assertions:** the per-test purge already scoped the log; do the action ->
  `await vendorAMock.verifyVendorReceivedPauseUpdate({ targetUuid, destinationTypes })`. If a path/verb
  isn't modeled, ADD a verify method - don't drop the assertion.
- **{{VENDOR_B}} assertions (integration):** after firing the order/import,
  `await vendorBMock.expectRecordResolved(id)` etc.; use exact-count helpers as the isolation backstop.
- **Before adding an `expect*` / `wait*` / `seed*` helper, grep this folder for one that already covers
  that vendor path, log predicate, or seed concern, and EXTEND it** with an optional parameter. A sibling
  helper beside an existing one needs a stated reason: two helpers over the same log predicate or seed
  drift, and only one of them gets updated.
- **No comments in support code. None.** Never explain `src/` or the backend here - no file names, props,
  render or query timing, test-id schemes, routing rules - and no provenance (tickets, commits,
  analysis), which belongs in the PR or the ticket. Give the helper a better name instead of a comment
  about it.

## Conventions & gotchas
- **One shared `test` fixture, and it lives HERE.** `fixtures.ts` is the single fixture object every
  spec - main area and integration alike - imports. Add a new fixture (page object, driver, helper)
  HERE, on that one object; Playwright fixtures are lazy, so a fixture no spec destructures costs other
  specs nothing. **NEVER fork a parallel or nested `fixtures.ts`** (e.g. under `tests/**`); ALL
  support/helpers live only in `playwright/support/`.
- {{VENDOR_A}}-path specs need the vendor A mock booted and `{{VENDOR_A_MOCK_URL_ENV}}` set; the
  integration harness needs the vendor B mock booted and `{{VENDOR_B_MOCK_URL_ENV}}` set. When a base URL
  is unset, purges no-op (safe) but `getLogs()`/`waitForLogs()` throw - so only assert against a mock you
  actually booted.
- `waitForLogs`/`verifyVendorReceived*`/`expect*` only see logs since the last purge - the `forEachTest`
  fixture purges once per test; don't add a second purge unless a spec needs mid-test isolation.
- Imports use the `~/playwright/...` and `~/cypress/...` aliases and this folder's own `utils.ts` -
  **never `~/utils`** (the app barrel ESM trap; see root doc). `backendApi`, `MockClient` and
  `IntegrationBackend` are deliberately dep-clean so the integration project can load them.
- `signInAsAdmin()` / admin-panel specs require the backend booted with `{{REVIEW_APP_FLAG}}`.
- **Call `server.seed(name, options)` with an options object whenever the seed reads one.** Omitting it
  sends no `options` key at all, so a seed doing `command_options.fetch('count', 3)` raises
  `undefined method 'fetch' for nil` and the seed 500s - the default is never reached. A legacy runner
  that always sent `{}` can make a seed that has only ever run there look like it has working defaults.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | Page objects [`../pageObjects/CLAUDE.md`](../pageObjects/CLAUDE.md) | Specs [`../tests/CLAUDE.md`](../tests/CLAUDE.md) | Integration harness [`../tests/integration/CLAUDE.md`](../tests/integration/CLAUDE.md)

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
