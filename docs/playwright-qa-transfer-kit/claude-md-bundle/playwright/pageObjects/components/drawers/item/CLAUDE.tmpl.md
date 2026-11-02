# drawers/item - Item creation + distribution-status drawers

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **createItem.ts** - `CreateItemDrawer`. <the multi-step creation flow: representative methods per step
  (type selection, configuration, duration via the composed section, submit/close) and the selection-table
  assertions it exposes; which queries/mutations it waits on>
- **<variant>.ts** - <any lighter variant of the creation drawer for another area>
- **itemDistributionStatus.ts** - `ItemDistributionStatusDrawer`. <the per-destination status panel:
  status validators, the failure-banner validator with its complement, the per-destination error
  validator, and the republish outcomes>

## How to use it
<which page/sidebar/header actions open each drawer; which section objects and factories it composes>

## Conventions & gotchas
- When a drawer's button shares its test id with a button in the page header, scope **every** locator
  in the drawer object to `#drawer-root`, or the page-header element wins the match.
- A status validator that takes backend enum types should map each one to the UI test id explicitly in
  the page object, not in the spec.
- Pass `false` to a banner/notification validator to assert the complement (the banner is absent).
- Large multi-step drawers have many conditional paths; some inputs use XPath indexing - extend carefully.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../../tests/main/item/`](../../../../tests/main/item/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
