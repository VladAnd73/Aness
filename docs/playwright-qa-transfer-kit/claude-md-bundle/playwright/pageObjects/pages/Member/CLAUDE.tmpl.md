# pages/Member - Member list & detail

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <list page object: class name, the `table: Tables` it holds and how `switchToView` reassigns it, the
  create/open/edit/remove/search methods, which GraphQL op search waits for, its `validateXList` helper,
  fixture name>
- <detail page object: class name, field validator, any sub-tables it holds; instantiated inline or fixture>

## How to use it
<how a spec reaches the screen (sidebar nav or goto) and which helper asserts list content inline>

## Conventions & gotchas
- Plural = list, singular = detail (see parent doc).
- **A detail page that renders two independently paginated/sorted tables side by side** holds one
  `ListTables` per table, each constructed with a `drawer` locator scoped to that table's container
  test id plus its own `pageSize`, so paging or sorting one table can be asserted not to touch the other.
- <test-id shape the detail fields use>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | drawer [`../../components/drawers/member/`](../../components/drawers/member/CLAUDE.md) | specs [`../../../tests/main/member/`](../../../tests/main/member/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
