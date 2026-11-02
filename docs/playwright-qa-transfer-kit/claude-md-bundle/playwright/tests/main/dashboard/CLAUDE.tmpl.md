# tests/main/dashboard - Dashboard specs

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <aggregated insights; filter by destination and by timeframe> | <seed path> | <yes/no> |

<POs: dashboard fixture, its filter drawers.>

## Patterns & gotchas
- `beforeEach`: seed -> `changeTenantTo` -> `goto(dashboardPage.url)`.
- Filters return the updated dashboard page object from their submit method; expected figures are
  computed inline from the seeded data and asserted against the rendered cards.
- <skipped filter tests and why; how the timeframe filter compares against seeded dates>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../../../pageObjects/pages/Dashboard/`](../../../pageObjects/pages/Dashboard/CLAUDE.md) | filter drawers [`../../../pageObjects/components/drawers/dashboard/`](../../../pageObjects/components/drawers/dashboard/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
