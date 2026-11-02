# tests/main/record - Record CRUD specs

Record create/list/detail/filter in the main area. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <create (factories, errors), list (views, filters, search, pagination), detail (fields, report-app link, state change asserted on detail AND list, sub-table isolation), record-variant layouts and their complements> | <seed paths> | <yes/no> |

<POs used.>

## Patterns & gotchas
- `beforeEach`: seed -> `changeTenantTo` -> `goto('/main/records')` (or a record UUID).
- Uses model factories for the minimal and the full record.
- List asserted inline via `validateXList(rows)` looped over view types. Filter expected results are
  computed by filtering the seeded array locally.
- When the seed's shape differs from the model the page object expects, map it in one file-level helper
  and reuse that helper in every new describe rather than re-inlining the mapping.
- **Search thresholds are per list** (one list may fire from 2 characters, another uses the component
  default) - read it from the component, don't carry a threshold across lists.
- **A filter drawer's Reset that applies immediately** leaves the drawer open - reset then CLOSE; don't
  also press Apply, the query won't refire. Unapplied changes being discarded on close is the complement
  worth asserting.
- When the pagination control renders nothing at a single page, the "not shown" case is a real assertion.
- A state change is asserted on the detail page **and** in the list.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../pageObjects/pages/Records/`](../../../pageObjects/pages/Records/CLAUDE.md) | drawer [`../../../pageObjects/components/drawers/record/`](../../../pageObjects/components/drawers/record/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
