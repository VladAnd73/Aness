# modals/entity - Entity-creation modals

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **createEntityModal.ts** - `CreateEntityModal`. <the type/tenant/group selector shown before creating
  certain entities: validators, setters, the per-entity create buttons (and what each returns), and
  the disabled-create negative check>
- **newEntityMenuModal.ts** - `NewEntityMenuModal`. The sidebar "Create new" floating menu that shows all
  entity types. `start*` methods open the corresponding drawer/flow and return it.
  `validateOfferedEntities(offered)` asserts the offered cards by title AND that every other entity in
  the full create-option list is absent - the complement is the point, since the menu is gated on the
  tenant's enabled areas.

## How to use it
<how each modal is reached and which selection gates creation>

## Conventions & gotchas
- A menu that returns drawer classes directly is tightly coupled to those drawers; if a create flow
  needs an intercept (e.g. an estimation query), the menu method installs it before opening the drawer.
- Assert a gated menu through the offered-set + complement oracle, not one card at a time; methods
  scoped to a container the current app version no longer renders are dead - assert through the live
  container and open the menu with a plain click (see [`../../CLAUDE.md`](../../CLAUDE.md)).
- When a card's description differs by tenant configuration, a single expected-text constant can hold
  only one of them - assert that card by title, not description.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../../tests/main/frozenDomain/topNavbar/createNew/`](../../../../tests/main/frozenDomain/topNavbar/createNew/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
