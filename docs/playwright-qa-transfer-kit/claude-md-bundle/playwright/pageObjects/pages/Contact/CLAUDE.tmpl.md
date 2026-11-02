# pages/Contact - Contact list & detail

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <list page object: class name, create/open/search/delete/row validators, the import helpers, fixture name>
- <detail page object: class name, field validator, edit entry point, instantiated inline>

## Conventions & gotchas
- Plural = list, singular = detail.
- When an opener method waits on a GraphQL op the method name does not hint at, say so here.
- Row selectors are `<rowId>_<column>` test ids and the row id is derived from the name cell, so the
  row must be on the rendered page: when the list paginates, search first.
- A summary "total" card that follows the active search/filters must be read with the list unfiltered.
- Row menus: when two context-menu variants exist, note which one this list uses and click the option
  rather than validating a root the variant does not render.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | drawer [`../../components/drawers/contact/`](../../components/drawers/contact/CLAUDE.md) | specs [`../../../tests/main/contacts/`](../../../tests/main/contacts/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
