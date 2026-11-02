# tests/main/adminPanel/ordersOverview - Order intake observability

The orders-overview list + drill-down. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | @smoke |
|------|-----------|--------|
| <spec file> | <list: columns, outcome badges, filters, search incl. wildcard sanitization, combined-filter AND semantics, one test per collapse rule, charts read back as counts, empty state, pagination; show page: header, sibling grouping strategies, warnings, related logs> | <count> |

<Seeds: the shared dashboard seed (which fixture chains it carries) and the dedicated pagination seed.>

<POs: the list and show page objects, both extending `AdminPanelBase`.>

## Patterns & gotchas
- **Give every collapse/filter rule its own test, and prove each one red on pre-fix backend code.**
  These specs guard SQL ordering and filter placement, where one wrong assertion order hides the rest:
  if the first assertion aborts, nothing behind it ever runs, so a green suite can cover a single
  rule. A test never shown red guards nothing - split until each rule fails on its own, and merge only
  rules that already carry their own demonstrated pre-fix failure. A chart or badge asserted
  `toBeVisible()` is superficial; assert the counts it renders, in the change that touches it.
- **When the index list is COLLAPSED (one canonical row per chain), assert against the collapsed view**,
  verified live against the shipped service, not assumed from the ticket.
- Assert each collapse rule against the one fixture that can fail on it (e.g. the only chain whose
  canonical row is older than its sibling and whose members sit in different tenants). Assert a chain
  size as the unfiltered value re-read under the filter, never as a literal.
- **Charts painted to a `<canvas>` have no readable DOM text.** Read the rendered series back through a
  page-object helper and compare it per key against the rows the matching filtered list shows - never
  against a literal. Charts computed on the collapsed scope are assertable evidence of the collapse.
- When the drill-down has more than one sibling-grouping strategy (exact key match vs a time-window
  fallback), cover each, plus an out-of-window exclusion.
- A section that is hidden entirely (not rendered) when empty is asserted hidden, not via an empty-state
  message.
- Setup: `userFlows.signInAsAdmin()` -> PO `open()`/`gotoAdmin`. Backend needs `{{REVIEW_APP_FLAG}}`. No
  Vite needed (server-rendered, backend-origin only).

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | sibling [`../autoOrderAttempts/CLAUDE.md`](../autoOrderAttempts/CLAUDE.md) | page objects [`../../../../pageObjects/pages/AdminPanel/`](../../../../pageObjects/pages/AdminPanel/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
