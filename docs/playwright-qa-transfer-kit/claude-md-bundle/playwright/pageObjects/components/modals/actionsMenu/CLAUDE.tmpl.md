# modals/actionsMenu - Row/card context menu

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **actionsMenuModal.ts** - `ActionsMenuModal`. The context menu opened by a row/card's action button.
  Methods: `validateVisible()`, `validateNotVisible()`, `clickOnOption(option)` (string or RegExp,
  targets `getByRole('menuitem')` in `getByTestId('context-menu-root')`).

## How to use it
Instantiated by the table classes (`ListTables`/`GridTables`) to drive edit/delete/archive/etc.
Accepts a `ContextMenuOptions` union (the per-entity action lists).

## Conventions & gotchas
- `clickOnOption` filters by text and won't error on a typo/missing option - pass the exact label.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | used by [`../../sections/list/`](../../sections/list/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
