# pages/AdminPanel - Server-rendered admin pages

Page objects for the server-rendered admin panel (Rails Administrate; NOT the SPA). Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **AdminPanelBase.ts** - `AdminPanelBase extends BasePage`. Base for all admin pages. `gotoAdmin(path)`
  navigates to the **backend origin** (`BACKEND_URL`, not the Vite `baseURL`) so the Devise session cookie
  stays same-origin; `expectSuccessFlash(message, { kind? })` - the custom `success` flash by default,
  `{ kind: 'notice' }` for the stock Administrate `notice` flash a generated create/update/destroy shows;
  `clickAndWaitForTurboResponse(target, { method, url })` for a Turbo link or form;
  `openNewForm(linkName, url)` (the list page's "New ..." link, waited like any Turbo link) and the
  `errorExplanation` getter (a rejected form's `#error_explanation`) for generated CRUD pages; the plain
  Administrate list helpers `listMain`, `listRows` and `searchList(term)` (the search box; waits for the
  GET carrying that `search` term and for Turbo to finish rendering it, so a list assert after it can't
  read the previous results); a nav helper that clicks a global-nav link leading to another origin and
  waits for the resulting cross-origin redirect.
- <one line per admin page object: class name, admin route, key methods, and whether it is a registered
  fixture or instantiated inline>
- <for list pages: filters, search, bulk actions, row badges; for show pages: the troubleshooting
  actions and any comment CRUD; for link/catalog pages: how a picker selects (by id, never display text)>

## How to use it
Admin specs sign in with `userFlows.signInAsAdmin()` (NOT `changeTenantTo`) and open pages via
`gotoAdmin`/`open`. Navigation is **Turbo**, not SPA routing.

## Conventions & gotchas
- **The backend must be booted with `{{REVIEW_APP_FLAG}}`**, which gates the passwordless test-only
  admin sign-in route. The admin panel runs on the backend origin (`{{BACKEND_PORT}}`), not the
  frontend's `{{FRONTEND_PORT}}`.
- **Turbo navigation:** gate on the destination control/page indicator after a click; never
  `waitForLoadState` alone (stale DOM). Where the next step fills a form, use
  `clickAndWaitForTurboResponse` so the fill can't land in Turbo's cached preview of the page. A Rails
  edit form submits as `POST` (`_method=patch`), so wait for `POST`. Confirm dialogs are caught with
  `page.once('dialog')`.
- **Selectize pickers** (searchable `BelongsTo` fields): click the `.selectize-input` next to the hidden
  `select`, type, then click the `.selectize-dropdown .option`; `selectOption` on the hidden `select` hangs.
- A "linked records" field on a show page: first require the field itself to render, then assert its
  contents, so an empty list can't pass on a page that failed.
- Forms that share a page are keyed by their `action` attribute suffix; paginated sub-lists use a
  `span.next a` selector (a numbered link also carries `rel="next"`).

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../tests/main/adminPanel/`](../../../tests/main/adminPanel/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
