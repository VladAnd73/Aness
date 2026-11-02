# sections/drawer - Drawer-embedded sections

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **<section>.ts** - <class name, the checkbox block it models, which drawer composes it>

## Conventions & gotchas
- A set-of-checkboxes setter toggles only the **diff** between the current and the desired set, so it
  is idempotent and never un-ticks a box the caller wanted kept.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page counterpart [`../blocks/`](../blocks/CLAUDE.md) | used by [`../../drawers/group/subunits/`](../../drawers/group/subunits/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
