# tests/main/orderManagement - Bundle order specs

Bundle activation, cancellation, and change. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <activation, cancel, change bundle, navigation from an item detail> | <seed path> | <yes/no> |

<POs used.>

## Patterns & gotchas
- Specs open the page via `orders?uuid=<instanceUuid>&token=<token>` - **both come from the seed's
  returned JSON** (a preview token; never minted via `eval`).
- <which section shows before vs after activation>
- A known-flaky case documented in-file is a flake note, not a green-by-matching target.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../../../pageObjects/pages/Orders/`](../../../pageObjects/pages/Orders/CLAUDE.md) | modal [`../../../pageObjects/components/modals/orders/`](../../../pageObjects/components/modals/orders/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
