# tests/main - Main-area specs (the `chromium` project)

Every main-area Playwright spec, organized by domain. This folder is the `chromium` project's
`testDir` in `playwright.config.ts` (`testMatch: **/*.spec.ts`). Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here (by area)
- **[`login/`](login/CLAUDE.md)** - sign-in (credentials, SSO) + role/tenant permissions.
- **[`sidebar/`](sidebar/CLAUDE.md)** - main-area sidebar navigation.
- **[`dashboard/`](dashboard/CLAUDE.md)** - dashboard insights + filters.
- **[`member/`](member/CLAUDE.md)** | **[`record/`](record/CLAUDE.md)** | **[`contacts/`](contacts/CLAUDE.md)**
  - main-area CRUD.
- **[`item/`](item/CLAUDE.md)** - item create/list/lifecycle/edit + bundles.
- **[`orderManagement/`](orderManagement/CLAUDE.md)** - bundle activation/cancel/change.
- **[`frozenDomain/`](frozenDomain/CLAUDE.md)** - a frozen domain. **Disabled: every describe there
  is `test.describe.skip`, so nothing in it runs.** Do not un-skip and do not add specs there -
  new coverage belongs in the domain folders above.
- **[`adminPanel/`](adminPanel/CLAUDE.md)** - the server-rendered admin panel.

## How to write a spec here
1. `import { test } from '~/playwright/support/fixtures';` - never from `@playwright/test`.
2. `beforeEach`: `server.seed('seeds/...')` -> (if the seed returns a tenant) `userFlows.changeTenantTo` ->
   `page.goto(pageFixture.url)`. The global auto-fixture already did `clean` + mock purge + admin login.
3. Body = a short list of page-object method calls, ending in an **end-state assertion via the UI**.
4. Tag `@smoke` if it should run on every PR.
5. Seed types come from the shared seed-types module - never declared inline.

## Conventions that bite here specifically
- **No locators in specs** - call page-object methods only.
- **No standalone "page/list renders" test** - assert list content inline via a `validateXList` helper.
- **Assert every effect** - primary state change (mandatory) + downstream {{VENDOR_A}}/{{VENDOR_B}} call
  at its mock transaction log; a banner alone is never enough.
- **Merge specs that share seed + setup**; keep them separate only for a genuinely different precondition.
- Async end-states: click the real UI, `{{DRAIN_COMMAND}}`, poll the UI - never a synthetic
  backend shortcut, never a Rails-console read.
- Skipped tests (`.skip`/`test.fixme`) in this tree are documented gaps (backend/mock/flaky) - keep the
  reason in the PR/ticket, and don't "green" a fixme by matching buggy behavior (surface the bug).

## Cross-links
Areas listed above. Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../pageObjects/CLAUDE.md`](../../pageObjects/CLAUDE.md) | integration harness [`../integration/CLAUDE.md`](../integration/CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
