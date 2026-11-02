# tests/main/frozenDomain/topNavbar/createNew - Create-entity flow specs

> **DISABLED - every `test.describe` in this folder is `test.describe.skip`.** This domain is out of
> scope going forward (the decision is recorded in {{TRACKER}}), so nothing under
> `playwright/tests/main/frozenDomain/**` executes in CI. **Do not un-skip a describe and do not add a
> spec here.** If a behaviour still matters, cover it under `playwright/tests/main/<domain>/`. The
> tables below remain the record of what this folder once exercised, not of what runs - treat every
> `@smoke` marking as historical.

Launching each creation flow from the sidebar "Create new" menu. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | @smoke |
|------|-----------|--------|
| <spec file> | <one row per entity type: open the create modal/drawer, validate content, cancel, disabled-without-selection> | <which tests> |

<the shared seed; POs used.>

## Patterns & gotchas
- Entity types that share one create modal are differentiated by entity name; a flow that navigates by
  URL instead of returning a drawer is asserted with `toHaveURL`.
- The "create is disabled" checks are the negative cases (nothing selected).
- Only the "validate modal content" tests are `@smoke`; cancel/disabled/proceed are regression-only.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | entity modals [`../../../../../pageObjects/components/modals/entity/`](../../../../../pageObjects/components/modals/entity/CLAUDE.md) | header [`../../../../../pageObjects/components/headers/`](../../../../../pageObjects/components/headers/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
