# tests/main/item/details/edit - Item edit specs

Editing an item after creation. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <content per destination, per-destination link and form fields, scope edits, schedule changes> | <seed paths> | <count / skipped> |

<PO: the editor drawer fixture.>

## Patterns & gotchas
- When the same editor serves two item variants, run each spec over both variants (two seeds), often via
  a nested `describe` over the seed.
- External services the editor calls (lookup and estimation services) are replaced by canned-response
  intercepts on `BasePage` (payloads in
  [`../../../../../data/fixtures/Responses/`](../../../../../data/fixtures/Responses/CLAUDE.md)), so no spec
  depends on a live third party. A spec that cannot avoid one locally stays skipped, with the reason.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | editor PO [`../../../../../pageObjects/pages/Item/`](../../../../../pageObjects/pages/Item/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
