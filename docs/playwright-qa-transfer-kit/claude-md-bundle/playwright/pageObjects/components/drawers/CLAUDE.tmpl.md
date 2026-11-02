# playwright/pageObjects/components/drawers - Slide-out panel objects

Page objects for the app's create/edit **drawers**. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`<entity>/`](<entity>/CLAUDE.md)** - <one line per subfolder: the drawer(s) it holds, e.g. create
  member, create item, create record, create contact, media library, edit post, the shared filters
  drawer, dashboard filters, group sub-drawers, the add/edit user drawer>

## Conventions & gotchas
- Drawers extend `BasePage`. A drawer is typically opened by a page/sidebar action that returns it, then
  its `fillOut*`/`configure*` method fills the form and its submit returns the resulting page.
- Drawer content lives in the `#drawer-root` portal - use `BasePage.openUnstableFloatingSection` /
  `waitForDrawerClosedOrReload` for the flaky open/close, and wait for the create/update mutation.

## Cross-links
Children listed above. Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
