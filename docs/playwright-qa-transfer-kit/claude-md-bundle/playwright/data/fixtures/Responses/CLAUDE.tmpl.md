# playwright/data/fixtures/Responses - Canned GraphQL mock payloads

Static response objects used to mock the app's own GraphQL queries via `page.route()`.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| File | Mocks |
|------|-------|
| `<name>.ts` | <one row per payload: the exported constant, which app query it stands in for, and what the UI renders from it> |

## How to use it
Consumed by the `intercept*GraphQuery` helpers on `pageObjects/BasePage.ts`, which `route.fulfill` these
payloads so the specs that render them don't depend on the live external estimation service.

## Conventions & gotchas
- These are snapshots of the backend contract with no type guard - a backend schema change can leave a
  spec green on stale data. The paired rendered-UI assertion is what proves correctness.
- A query the spec doesn't explicitly mock falls through to the real backend.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | intercepts defined in [`../../../pageObjects/CLAUDE.md`](../../../pageObjects/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
