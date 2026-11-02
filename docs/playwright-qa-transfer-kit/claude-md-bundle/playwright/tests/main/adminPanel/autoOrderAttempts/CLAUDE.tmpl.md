# tests/main/adminPanel/autoOrderAttempts - Automatic-order pipeline observability

The automatic-order attempts list + drill-down + a sibling write-back show page. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | @smoke |
|------|-----------|--------|
| <spec file> | <what it exercises: grouping rule of the list, filters, search, chart-vs-table divergence, empty state, pagination> | <count> |

<Seeds: which seed (shared with `ordersOverview/`) and which fixture groups it carries; the separate pagination seed.>

<POs: the admin page objects used, all extending `AdminPanelBase`; why a resource with no standalone
index lives here rather than in its own folder.>

## Patterns & gotchas
- **When the shipped panel diverges from the ticket's stated shape, assert the shipped shape** - verified
  live against the controller/service/view code, not assumed from the ticket - and record the divergence
  here (e.g. grouped rows instead of one row per attempt, two drill-down surfaces keyed differently).
- Pin time-window and tie-break rules exactly as the code implements them (a calendar-day window is not
  a +/-24h window; "earliest wins" is not "most recent"), and give each such rule an explicit assertion.
- A chart that totals raw events while the table shows collapsed groups is an intentional divergence;
  assert both numbers, not their equality.
- Setup: `userFlows.signInAsAdmin()` -> PO `open()`/`gotoAdmin`. Backend needs `{{REVIEW_APP_FLAG}}`. No
  Vite needed.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | sibling [`../ordersOverview/CLAUDE.md`](../ordersOverview/CLAUDE.md) | page objects [`../../../../pageObjects/pages/AdminPanel/`](../../../../pageObjects/pages/AdminPanel/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
