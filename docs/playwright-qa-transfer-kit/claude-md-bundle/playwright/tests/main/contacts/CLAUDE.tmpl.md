# tests/main/contacts - Contact CRUD specs

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <create (full, mandatory-only, from an item detail), edit + delete, CSV import> | <seed path> | <yes/no> |

<POs used.>

## Patterns & gotchas
- `beforeEach`: seed -> build the expected object with a model factory -> `changeTenantTo` -> navigate.
- Environment-dependent values (e.g. a seeded location vs a local fallback) are baked into the expected
  object before the form is filled.
- **When a seed creates a random NUMBER of rows, never assert a row count.** Search for the row by name
  and assert the system-computed total instead, read with the list unfiltered.
- Expected labels come from the seed's values through the shared normaliser, never hard-coded; a
  replacement value is pinned so an edit is always a real change.
- Edit and delete stay separate tests: each mutates the shared seed, and a merged failure could not say
  which operation broke. The global `clean` seed gives each its own data, so neither depends on order.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../pageObjects/pages/Contact/`](../../../pageObjects/pages/Contact/CLAUDE.md) | drawer [`../../../pageObjects/components/drawers/contact/`](../../../pageObjects/components/drawers/contact/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
