# playwright/pageObjects/components - Reusable UI component objects

Page objects for UI pieces shared across pages: drawers, modals, sections (incl. tables), headers, and
the app sidebar. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **AppSidebar.ts** - `AppSidebar`: the main app navigation. `navigateTo*` methods (Records, Items,
  Contacts, ...) return the destination page object; `startCreate*` open the create flows;
  `clickOnLogo()`; `switchLanguage(language)` / `validateLanguage(language)` (the switcher pill - only
  readable while the user menu is open) / `verifyLanguageInPage(language)` (the rendered nav links, the
  oracle for "the UI is actually translated"); `validateCreateNewEntities(offered)` (the offered-set +
  complement assertion for the create menu). Registered as the `appSidebar` fixture.
- **ListComponents.ts** - the `listTypes` const (`['Grid', 'List']`) and derived `ListTypes` type,
  used by list-view page objects and the `listTypes.forEach(...)` view-variant loops in list specs.
- **[`drawers/`](drawers/CLAUDE.md)** - slide-out create/edit panels (one subfolder per entity, plus the
  shared filters drawer and dashboard filters).
- **[`modals/`](modals/CLAUDE.md)** - dialogs (confirmation, entity-creation, actions menu, order
  bundle change).
- **[`sections/`](sections/CLAUDE.md)** - in-page building blocks, most importantly the
  `Tables`/`ListTables`/`GridTables` list machinery.
- **[`headers/`](headers/CLAUDE.md)** - wizard/dialog headers.

## How to use it
Most component objects are **instantiated inline** (`new CreateMemberDrawer(page)`) and are usually
returned by a page object's action (`membersPage.startAddMember()` -> `CreateMemberDrawer`) rather than
constructed by the spec directly. Only a few are registered as fixtures (e.g. `appSidebar`,
`filtersDrawer`, and a handful of frequently reused drawers / sidebars).

## Conventions & gotchas
- **Do NOT open the sidebar create menu with `BasePage.openUnstableFloatingSection`.** Its stability
  probe waits for Headless-UI **v1**'s `#headlessui-portal-root`, which this app no longer renders, so
  the probe always fails and the helper reloads the page (up to 4 times, with an un-awaited `reload()`)
  - leaving the menu closed and the assertion racing a reload. Use a plain `click()` and let the option
  assertions do the waiting. The same caveat applies to any other Headless-UI v2 menu.
- All extend `BasePage`; selectors are private getters via `getByTestId`/`getByRole`.
- Create-drawers wait for their mutation (`waitForGraphQLResponse('mutation createX')`) before/after
  submit, and their `fillOut*AndSubmit` returns the resulting list page.
- Reuse: if a drawer/modal is missing a method your spec needs, add it here - don't build a parallel
  component object.

## Cross-links
Children: [`drawers/`](drawers/CLAUDE.md) | [`modals/`](modals/CLAUDE.md) | [`sections/`](sections/CLAUDE.md) | [`headers/`](headers/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
