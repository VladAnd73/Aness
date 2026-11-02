# pages/Group - Frozen-domain group screens

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here (root files + subfolders)
- <list page object: class name, the `table: Tables` it holds, view switch, open/create/edit/search
  methods (and the GraphQL op search waits for), count validator, fixture name>
- <detail page object: class name, the integration-status section validators and the sub-table it scopes>
- **[`Analytics/`](Analytics/CLAUDE.md)** | **[`items/`](items/CLAUDE.md)** |
  **[`subunits/`](subunits/CLAUDE.md)** - group sub-screens.

## Conventions & gotchas
- <search minimum length; which sections are absent depending on integration state>
- A table that lives inside one block of a page is scoped to that block's locator, not the page root
  (the sub-table pattern).

## Cross-links
Children: [`Analytics/`](Analytics/CLAUDE.md) | [`items/`](items/CLAUDE.md) | [`subunits/`](subunits/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../tests/main/frozenDomain/`](../../../tests/main/frozenDomain/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
