# components/modals - Dialog objects

Page objects for modal dialogs. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`<modal>/`](<modal>/CLAUDE.md)** - <one line per subfolder: e.g. generic confirm/cancel dialog,
  entity-creation type picker + the sidebar "Create new" menu, the row/card context menu, the bundle
  change modal>

## Conventions & gotchas
- All extend `BasePage`, instantiated inline, selectors via `getByTestId`/`getByRole`. Modals are
  collaborators - a table's `deleteItem()` opens `ConfirmationModal`; the sidebar create flow opens
  `NewEntityMenuModal` -> a drawer.

## Cross-links
Children listed above. Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
