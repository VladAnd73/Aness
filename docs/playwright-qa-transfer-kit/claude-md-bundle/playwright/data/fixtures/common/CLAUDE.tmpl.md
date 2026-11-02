# playwright/data/fixtures/common - Shared test constants

Small shared constants for test orchestration. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **common.ts** - `DELETE_MUTATIONS` and `UPDATE_MUTATIONS` (GraphQL operation-name lists used to wait
  for the right destructive/update request), and `TableViewTypes` (`'Grid' | 'List'`).

## Conventions & gotchas
- The mutation-name strings are hard-coded; if a backend mutation is renamed, waits keyed on these
  silently stop matching - keep them in sync.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
