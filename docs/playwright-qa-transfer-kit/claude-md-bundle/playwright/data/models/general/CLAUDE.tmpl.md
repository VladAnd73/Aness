# playwright/data/models/general - Platform-wide constants

<one line: shared constants used across all domains>. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **general.ts** - <the platform-wide constant map (e.g. one entry per destination: UI label, backend
  type, identifier) and its derived identifier type; who consumes it>

## Conventions & gotchas
- The constant map is `as const` (immutable) - override via factory overwrites, don't mutate it.
- <any naming quirk where a UI identifier maps to a differently-named backend type; record it here so
  nobody "fixes" the mapping>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
