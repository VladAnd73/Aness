# playwright/prodSmoke - post-deploy production smoke

A read-only smoke that runs **against production** after a backend production deploy. It proves the
public read API is up and that a fixed canary page of the report app renders every section from it.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **readApiSmoke.spec.ts** - two tests:
  1. *read API is up and issues the canary tenant a credential* - `GET {API}/status` is 200 and the
     tenant-credentials read is 200 with exactly one non-empty credential. No browser.
  2. *canary report renders every section from the read API* - opens the canary report page and first
     asserts that every required read-API call answered, with its expected body shape and 2xx, so a
     read-API failure names the failing endpoint. It then asserts <the report's required sections, one
     per line>, no error state or error toast, and finally that no late read failed and no write was
     attempted.
- **readApi.ts** - `ReadApi`, the two API checks of test 1. Both reads use `maxRedirects: 0`: an unknown
  read-API path answers **302**, and a followed redirect would turn a broken `/status` into a 200.
- **prodSmokeTarget.ts** - hosts and canary ids, each overridable by env (below).
- The page object is shared: [`../pageObjects/pages/ReportApp/`](../pageObjects/pages/ReportApp/CLAUDE.md).

## How it is wired
- **Own config, outside the regular suite:** `{{PROD_SMOKE_CONFIG}}` (repo root) has
  `testDir: ./playwright/prodSmoke`, one `prod-smoke` chromium project (desktop viewport, `en-US`),
  `retries: 1`, `trace: retain-on-failure`, **no `webServer`, no `globalSetup`, no `playwright/.env`**.
  Because this folder is outside `playwright/tests`, neither the default `playwright.config.ts` projects
  nor `{{SHARD_SPLITTER_SCRIPT}}` ever collect it.
- **Specs import `test` from `@playwright/test`, NOT from `~/playwright/support/fixtures`.** The suite
  fixture's auto `forEachTest` seeds `clean`, purges mocks and creates/logs in an admin against a test
  backend - none of which may run here.
- **CI:** `.github/workflows/{{PROD_SMOKE_WORKFLOW}}`, triggered by `repository_dispatch`
  (`event_type: prod-smoke`, sent by the backend repo's deploy workflow after a production deploy) or by
  hand with `workflow_dispatch`. Its `run-name` carries a `correlation_id` from the payload (built from
  the deployed commit and release versions), or `manual-{run_id}` for a manual run; the backend finds the
  run by that string. `client_payload` = <the payload fields: commit, correlation id, release versions,
  deployment ids, split-version flag>. A split-version payload does **not** fail the run: it is shown as
  a warning and in the job summary, and the smoke runs anyway. On failure the report + traces are
  uploaded as an artifact and the `notify` step runs (a placeholder until an alert channel is chosen).
  `repository_dispatch` always runs the workflow from `{{DEFAULT_INTEGRATION_BRANCH}}`.

## Env overrides (defaults are the locked production values)
| Var | Default |
|---|---|
| `PROD_API_URL` | {{PROD_SMOKE_TARGET}} (the read-API host the production report app calls) |
| `PROD_REPORT_URL` | <the production report-app host> |
| `CANARY_TENANT_UUID` | {{CANARY_IDS}} (a dedicated test tenant) |
| `CANARY_REPORT_ID` | {{CANARY_IDS}} (one fixed canary report) |

`PROD_API_URL` steers only test 1: the report app has its read-API host built into its bundle.

## Running it
```bash
yarn playwright install chromium
yarn playwright test -c {{PROD_SMOKE_CONFIG}}
```
No backend, no Vite, no `playwright/.env`. It reaches production, so run it only when you mean to.

## Rules for this folder
- **Production is read-only.** GET only. The page object aborts any other method in the browser and
  fails the test if one was attempted. Never call a write endpoint, never open anything that triggers a
  live third-party call, never click a "Refresh" that re-fetches from a vendor, never use
  `--disable-web-security`.
- **Assert rules, not canary numbers.** The canary's data is real and can drift; assert "a number is
  shown", "at least one entry", "the section renders" - never exact values.
- **Read-API checks run before the section checks.** A section check that fails because a read failed
  would say "canary data changed?" instead of naming the endpoint.
- **A data-presence failure says "canary data changed?"** so a red caused by someone editing the canary
  is quick to tell apart from a code break.

## What the canary actually shows (checked live <date>)
<one bullet per section: what the canary renders today, and which stricter rule (e.g. "> 0") is NOT
asserted because the canary's real data cannot satisfy it, plus when it could be added.>

## Proven red
Every rule has gone red on its own against production, with the spec unchanged or with a throwaway route
stub. <List each red case: a nonexistent report id; an unreachable read-API host (test 1) and a blocked
read-API host in the browser (test 2); `/status` answering 302; a tenant with no credential; each
required read answering 500, invalid JSON, never answering, or `200` with an unexpected body; a late 500
read; a page-initiated write. When the check order changes, re-prove and record which cases now name the
endpoint.>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../pageObjects/pages/ReportApp/`](../pageObjects/pages/ReportApp/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
