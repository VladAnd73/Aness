# tests/main/member - Member CRUD specs

Member list, creation and detail in the main area. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <list (both views, pagination, search, row actions), create (all fields, mandatory-only, errors), detail (sub-table isolation)> | <seed paths> | <yes/no> |

<POs: list fixture, create drawer fixture; the detail page object is instantiated inline.>

## Patterns & gotchas
- `beforeEach`: seed -> `changeTenantTo(tenant)` -> `goto` the list or detail route.
- **Reference for the reusable-list-helper rule:** the list's `validateXList(rows)` asserts list content
  inline and is looped over both view types - no standalone "list renders" test.
- Rows are read from the seed and asserted inline.
- Two tables on one detail page: each is a `ListTables` scoped via its container locator + `pageSize`;
  assert that paging/sorting one leaves the other untouched, in both directions.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../pageObjects/pages/Member/`](../../../pageObjects/pages/Member/CLAUDE.md) | drawer [`../../../pageObjects/components/drawers/member/`](../../../pageObjects/components/drawers/member/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
