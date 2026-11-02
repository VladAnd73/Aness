# tests/main/adminPanel/failedItems - Failed-items repair dashboard

The admin dashboard for triaging/repairing/republishing failed item publications. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | @smoke |
|------|-----------|--------|
| <spec file> | <one row per spec: roster render, filters, tenant scope, pagination, assignment, external links, comments, badges, bulk actions, show page, unpark; mark the ones that assert at the {{VENDOR_A}} mock> | <count, plus fixme count> |

<Seeds: the rich dashboard seed (which states it carries) and the dedicated pagination seed; which seed
option enables outbound distribution.>

<POs: the list and show page fixtures, the inline show pages, `vendorAMock`.>

## Patterns & gotchas
- Setup: `userFlows.signInAsAdmin()` -> list page `open()`. Backend needs `{{REVIEW_APP_FLAG}}`.
- **Republish -> distribution assertion:** `vendorAMock.purgeLogs()` -> action ->
  `vendorAMock.verifyVendorReceivedRepublish({ itemName, destinationTypes })`; needs {{MOCK_TOOL}} on
  `{{VENDOR_A_MOCK_PORT}}` + `{{VENDOR_A_MOCK_URL_ENV}}`.
- **Bulk vs per-row scope:** state here exactly which rows each bulk action touches (the active filters,
  any fallback time window when unfiltered) versus the per-row actions on the show page, and give the
  filtered variant of each bulk action its own test proving it stays inside the filter.
- **Async draining:** a bulk action that fans out one job per row with spacing between them must be
  drained before the next test - wait for a drain marker (e.g. the last row leaving the list).
- `test.fixme` flags track real gaps - reasons live in the PR/tickets; don't green them by matching
  buggy behavior.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../../pageObjects/pages/AdminPanel/`](../../../../pageObjects/pages/AdminPanel/CLAUDE.md) | mock helper [`../../../../support/`](../../../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
