# drawers/filters - Shared filter drawer

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **filters.ts** - `FiltersDrawer`: <the filter panel reused across several entity lists; its per-filter
  select methods, the per-entity appliers (each returns the matching list page), reset/cancel/close, and
  validators>

## How to use it
<fixture name and the list-page methods that return it; which entity types change its title/sections>

## Conventions & gotchas
- Filter test-id names may be inconsistent - check the actual getter before adding one. Where a UI label
  and the backend enum differ (e.g. a UI phase name vs its backend value), the select method accepts
  either spelling so a seeded enum value can be passed straight through.
- **Sections of the drawer can be conditional** on the route and on tenant settings; before writing a
  spec, check the section is reachable from the route and tenant you seed.
- When the app names a button per entity type (`<entity>-reset-button`), the getter `.or()`s the
  variants - add the new prefix there when another entity type starts using the drawer, rather than
  forking a second method.
- **Reset applies immediately** and does NOT close the drawer. Follow `resetFilters()` with
  `closeDrawer()`; following it with an apply-and-wait helper hangs, because the list query already
  refired and won't refire for an unchanged filter set.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | <list pages + specs that use it>.

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
