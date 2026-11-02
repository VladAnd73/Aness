# tests/main/login - Authentication & permissions specs

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <credential sign-in; SSO with email normalization; role x tenant visibility and access> | <seed paths> | <yes/no> |

<POs used.>

## Patterns & gotchas
- These specs `clearSession()` first (the global auto-fixture logs in as admin otherwise) and
  `goto('/login')` - they do NOT `changeTenantTo`.
- Users are created dynamically through `userFlows` helpers (create with role, assign to tenants); the
  SSO mock seed is keyed to the created email.
- Role gating is asserted via UI interactability (e.g. whether the tenant switcher is visible and
  usable), not a raw permission read.
- **Access DENIAL is not e2e-testable when the review backend auto-signs-in an admin**; cover denial at
  the backend layer instead.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../../../pageObjects/pages/Login/`](../../../pageObjects/pages/Login/CLAUDE.md) | auth flows [`../../../support/`](../../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
