# tests/main/frozenDomain/items/list - Item list action specs

> **DISABLED - every `test.describe` in this folder is `test.describe.skip`.** This domain is out of
> scope going forward (the decision is recorded in {{TRACKER}}), so nothing under
> `playwright/tests/main/frozenDomain/**` executes in CI. **Do not un-skip a describe and do not add a
> spec here.** If a behaviour still matters, cover it under `playwright/tests/main/<domain>/`. The
> tables below remain the record of what this folder once exercised, not of what runs - treat every
> `@smoke` marking as historical.

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <archive / unarchive / pause / resume on the app-wide list, and the same within a group> | <seed path> | <yes/no> |

<PO: the group-scoped item list fixture.>

## Patterns & gotchas
- A seed returning items across every phase lets each spec `sample()` one by phase.
- Expected end-states mirror the app's own transition rules (which phase unarchive or resume lands in,
  depending on the item's history and dates); each action updates the expected object and re-asserts
  the phase.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../../../../../pageObjects/pages/Group/items/`](../../../../../pageObjects/pages/Group/items/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
