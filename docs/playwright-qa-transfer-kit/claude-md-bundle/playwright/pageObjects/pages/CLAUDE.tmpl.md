# playwright/pageObjects/pages - Screen/route page objects

One class per app route/screen - the main entry objects a spec drives. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here (root files + subfolders)
- **Home.ts** - `Home` (route `/main`): landing page; `validateVisible()`, the "start a new item" entry
  points, `validateNoItems()`.
- **Login.ts** - `Login`: a **minimal validation-only stub** (form getters + `validateVisible()`). The
  REAL login flow object is [`Login/login.ts`](Login/CLAUDE.md) (registered as the `loginPage` fixture).
- **[`Account/`](Account/CLAUDE.md)** - the ungated `/account/**` screens (user list + user details).
- **[`AdminPanel/`](AdminPanel/CLAUDE.md)** - server-rendered admin pages (Turbo-driven, backend origin).
- **[`Member/`](Member/CLAUDE.md)** | **[`Item/`](Item/CLAUDE.md)** | **[`Dashboard/`](Dashboard/CLAUDE.md)** |
  **[`Contact/`](Contact/CLAUDE.md)** | **[`Login/`](Login/CLAUDE.md)** | **[`Orders/`](Orders/CLAUDE.md)** |
  **[`Group/`](Group/CLAUDE.md)** | **[`Records/`](Records/CLAUDE.md)** - domain screens.
- **[`ReportApp/`](ReportApp/CLAUDE.md)** - the report app, a separate app on its own host (not a
  {{PROJECT_NAME}} route); used by the production smoke.

## How to use it
- Pages registered as fixtures (see root [`../../CLAUDE.md`](../../CLAUDE.md)) are destructured in the
  spec; navigate with `await page.goto(pageFixture.url)` when the class exposes a `url`, then
  `validateVisible()`.
- List pages (Items, Records, Groups, Members) hold a `table: Tables` and expose a
  `validateXList(items)` helper - assert list content inline via it (no standalone "list renders" test).
- Actions return the next page object; chain screens rather than re-instantiating.

## Conventions & gotchas
- **Filename casing duplication is intentional**: plural = list page, singular = detail page
  (`Member/members.ts` list vs `Member/member.ts` detail; likewise Contact, Records). And there are
  **two** `Login` classes - the root stub vs the real `Login/login.ts`.
- All extend `BasePage` (admin pages extend `AdminPanelBase`, which extends `BasePage`).

## Cross-links
Subfolders listed above. Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
