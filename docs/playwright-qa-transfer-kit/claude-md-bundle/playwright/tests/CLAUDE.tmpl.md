# playwright/tests - The specs

All Playwright specs live here. `playwright.config.ts` sets `testDir: ./playwright/tests` and splits
the suite into two projects by subfolder. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here (two projects)
- **[`account/`](account/CLAUDE.md)** - the `/account/**` specs (currently `users/`). Part of the
  **`chromium`** project. Separate from `main/` because these routes are ungated: every persona reaches
  them, so a spec here must not assume a main-area member context.
- **[`auth/`](auth/CLAUDE.md)** - the public authentication routes (`/login`, `/forgot-password`,
  `/reset-password`). Also **`chromium`**. <If an older auth spec still lives elsewhere, name it here;
  new auth coverage goes in this folder.>
- **[`main/`](main/CLAUDE.md)** - the main-area specs, the default **`chromium`** project (which
  collects all of `tests/` except `integration/`). This is what a plain `yarn playwright test` and the
  standard CI smoke/regression run exercise. Organised by domain <list the domain folders, e.g. login,
  sidebar, dashboard, member, record, contact, item, orderManagement, adminPanel>.
- **[`integration/`](integration/CLAUDE.md)** - the integration-as-user harness, the **`integration`**
  project (`--project=integration`). Drives the REAL {{INTEGRATION_PIPELINE}} through the backend ->
  message queue -> integration gateway -> a {{VENDOR_B}} mock, and needs a full out-of-band stack. It
  imports the same shared `test` fixture as everything else.

The two projects are isolated by the integration `testDir` plus chromium's matching `testIgnore`, so a
`--project=chromium` run never collects the integration specs (which would otherwise try to seed against
a stack that isn't booted) and vice-versa.

**A new spec folder here runs under `chromium` automatically - keep it that way.** `chromium` collects
all of `tests/` and ignores only `integration/`, so adding a folder needs no config change. Narrowing it
to specific folders instead would make a spec at any other path collected by **no** project: no conflict,
no warning, exit 0, and the regression net quietly loses it. Adding a second self-scoped project (as
`integration` is) means adding its path to chromium's `testIgnore` in the same change.

## Convention
Every spec imports `test` from `~/playwright/support/fixtures` - the single shared fixture. There is
**NO `support/` subfolder under `tests/**`**; all infra (fixtures, seeding, helpers) lives only in
[`../support/`](../support/CLAUDE.md). Don't fork a parallel fixture next to the specs.

## What the spec set must cover
Every AC of the ticket, and of every ticket it links to, gets a spec that asserts the behaviour. An AC
naming a population (one record kind, one role, one destination) also claims the rest is unaffected -
add the complement scenario; if no seed can produce it, extend the seed. Full rules:
[`.cursor/rules/testing.mdc`](../../.cursor/rules/testing.mdc) -> "What to Assert".

## Where to add a spec
- An ungated shared surface (`/account/**`, the auth routes) -> `account/<area>/` or `auth/`. Read
  [`account/CLAUDE.md`](account/CLAUDE.md) / [`auth/CLAUDE.md`](auth/CLAUDE.md) first.
- A main-area feature -> under `main/<domain>/`. Read [`main/CLAUDE.md`](main/CLAUDE.md) for the
  per-domain layout and the spec-writing checklist.
- An integration-driven end-to-end flow -> under `integration/`. Read
  [`integration/CLAUDE.md`](integration/CLAUDE.md) first - those specs use the **same shared `test`
  fixture** (plus integration driver/mock fixtures), committed seeds, and an out-of-band stack.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | account [`account/CLAUDE.md`](account/CLAUDE.md) | auth [`auth/CLAUDE.md`](auth/CLAUDE.md) | main area [`main/CLAUDE.md`](main/CLAUDE.md) | integration [`integration/CLAUDE.md`](integration/CLAUDE.md) | page objects [`../pageObjects/CLAUDE.md`](../pageObjects/CLAUDE.md) | infra [`../support/CLAUDE.md`](../support/CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
