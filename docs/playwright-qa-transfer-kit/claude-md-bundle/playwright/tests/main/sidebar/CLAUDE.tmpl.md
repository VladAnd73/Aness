# tests/main/sidebar - Main-area navigation

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file - describe> | <navigate to each area via sidebar links; app-logo destination; the create menu's offered entity set> | <seed path> | <yes/no> |
| <spec file - describe> | <switching to each non-default language translates the sidebar, with no page reload> | <seed path> | <yes/no> |

<POs used.>

## Patterns & gotchas
- `beforeEach`: seed -> `changeTenantTo` -> `goto(homePage.url)`.
- Each `appSidebar.navigateTo*()` returns the destination page object; the test then calls
  `validateVisible()` on it (asserting the destination, not just that a link exists).
- **Read where a link resolves from the app code before asserting it.** E.g. a logo that resolves by user
  type: the suite's auto-fixture logs in as an admin, so assert the admin destination for every tenant type.
- A validator that reads an element inside a dropdown only works while that dropdown is open; assert the
  default state through a validator that reads the always-rendered elements instead.
- **Open a menu with a plain click** unless the components doc says it needs the unstable-floating
  helper - see [`../../../pageObjects/components/CLAUDE.md`](../../../pageObjects/components/CLAUDE.md).
- The frozen-domain sidebar specs in [`../frozenDomain/sidebar/`](../frozenDomain/sidebar/CLAUDE.md) are
  skipped; their live coverage is here.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | sidebar PO [`../../../pageObjects/components/CLAUDE.md`](../../../pageObjects/components/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
