# playwright/pageObjects - Page Object Model

Every selector and UI interaction lives here, behind page-object methods. Specs never contain locators;
they call methods on these classes. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **BasePage.ts** - the base class **every** page object extends. Provides shared machinery so
  subclasses don't reinvent it.
- **[`pages/`](pages/CLAUDE.md)** - one class per route/screen (Items, Record, Groups, AdminPanel, ...).
  The main entry objects a spec drives.
- **[`components/`](components/CLAUDE.md)** - reusable pieces used across pages: drawers, modals,
  sections (incl. the table classes), headers, and the app sidebar.

## BasePage - what subclasses get
- **Constructor** `constructor(page: Page)` - stores `page` and waits for load. Every subclass calls
  `super(page)`.
- **GraphQL waits** - `waitForGraphQLRequest(queryName, timeout?)` /
  `waitForGraphQLResponse(queryName, timeout?)`: gate an action on the specific `/api/graphql`
  operation it triggers (e.g. `waitForGraphQLResponse('mutation createMember')`).
- **Floating-UI stability** - `openUnstableFloatingSection(...)` and `waitForDrawerClosedOrReload(...)`:
  retry/reload wrappers for flaky drawer/menu portals (`#drawer-root`, `#headlessui-portal-root`).
- **Multi-element waits** - `waitForAnyElementToBeVisible/Attached([...], timeout?)`.
- **Async write-back waits** - `reloadUntil(assertion, timeout?)`: reload-and-retry until an assertion
  passes, for pages that don't live-update when a background import lands.
- **Query intercepts** - `intercept*GraphQuery` helpers (e.g. the estimation queries and
  the restrictions query): `route.fulfill` the canned payloads from
  [`../data/fixtures/Responses/`](../data/fixtures/Responses/CLAUDE.md) so estimation-dependent specs
  don't hit the live external estimation service.
- **Small helpers** - `onLocatorSelectOption(...)`, `getTextBoxWithTestId(testId)`.

## How to use it (the POM pattern in THIS suite)
- **Extend `BasePage`, export `default`:**
  ```typescript
  export default class Widget extends BasePage {
    constructor(page: Page) { super(page); }
    private get saveButton(): Locator { return this.page.getByTestId('save'); }
    async save(): Promise<void> { await this.saveButton.click(); }
  }
  ```
- **Selectors are private getters** (or private methods when they need a `rowId` argument - getters
  can't take params; see `components/sections/list/`). Prefer `getByRole` / `getByTestId`.
- **Reuse via fixtures.** A PO registered in `support/fixtures.ts` is pulled in by destructuring
  (`async ({ itemsPage }) => ...`); others are `new`-ed inline. Never fork a second copy of an
  existing PO - add a method to the real one.
- **Before adding a `validate*` / `verify*` / `expect*` method, grep this folder (and `BasePage`) for one
  that already asserts the same locator or concern, and EXTEND it** with an optional parameter - an
  optional scope/tab, a `poll?` or strict mode. A sibling helper beside an existing one needs a stated
  reason: two methods over the same locator drift the moment the page changes, and only one of them
  gets updated.
- **No comments in a page object. None.** Never explain `src/` here - no component or file names, props,
  render or query timing, test-id schemes, routing rules. Provenance (tickets, commits, root-cause
  analysis) belongs in the PR or the ticket, conventions in this `CLAUDE.md`. Rename the thing rather
  than adding a comment about it, and don't move the prose into a method or variable name instead.
- **Navigation methods return the next PO** (`startAddItem()` -> `CreateItemDrawer`), so specs
  chain screens fluently.

## Conventions & gotchas
- **Assert the rule, not the value the spec handed you.** Where a field is system-derived (an
  integration, a URL template, a precedence rule), match the documented shape -
  `toHaveValue(/^https:\/\/[^/]+\/template\//)`, not `toHaveValue(theStringTheSpecFilled)`. See the
  "What to Assert" section of the repo's testing rules (in the reference project,
  `.cursor/rules/testing.mdc`).
- Imports use `~/playwright/*` and `~/cypress/common/types/*`. **Never `~/utils`** (app barrel ESM trap
  - root doc). The one deliberate exception seen in the suite: `components/modals/orders/` imports a
  type from an app page's `types` module (a type-only leaf, not the util barrel).
- Intercept helpers use hard-coded mock numbers - a spec asserting exact estimated values is coupled to
  those fixtures.
- `openUnstableFloatingSection` may reload the page up to 4 times - don't assume in-page state survives it.

## Cross-links
Children: [`pages/`](pages/CLAUDE.md) | [`components/`](components/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
