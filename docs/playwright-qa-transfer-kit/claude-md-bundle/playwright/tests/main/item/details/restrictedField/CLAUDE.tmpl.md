# tests/main/item/details/restrictedField - Restricted-field visibility specs

Role-gated visibility of a restricted field across list, detail, edit, dashboard, and creation. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Role | Exercises | Seeds | @smoke |
|------|------|-----------|-------|--------|
| <spec file> | <role from {{ROLES}}> | <surfaces where the field is hidden or shown> | <seed path> | <count> |

<POs used.>

## Patterns & gotchas
- Role is set via a `userFlows` login-with-role helper vs the default admin login. Specs that share a seed
  but have a different **role precondition** are a legitimate reason to keep separate spec files.
- Presence is asserted via `hasRestrictedFieldVisible()` on each surface - the actual rendered UI - in both
  directions.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../../../pageObjects/pages/Item/`](../../../../../pageObjects/pages/Item/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
