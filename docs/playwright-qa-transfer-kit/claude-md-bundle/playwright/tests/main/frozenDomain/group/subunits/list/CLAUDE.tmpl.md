# tests/main/frozenDomain/group/subunits/list - Subunits list specs

> **DISABLED - every `test.describe` in this folder is `test.describe.skip`.** This domain is out of
> scope going forward (the decision is recorded in {{TRACKER}}), so nothing under
> `playwright/tests/main/frozenDomain/**` executes in CI. **Do not un-skip a describe and do not add a
> spec here.** If a behaviour still matters, cover it under `playwright/tests/main/<domain>/`. The
> tables below remain the record of what this folder once exercised, not of what runs - treat every
> `@smoke` marking as historical.

<one line on what the subunits list specs cover>. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <one row per spec: create, edit/delete, warnings, row actions, detail sections, filters, search, pagination threshold, integration status, document drawer> | <seed path (count)> | <yes/no> |

<POs used.>

## Patterns & gotchas
- `beforeEach`: seed -> `goto` the group's subunits route; validate the sidebar group name.
- Pagination thresholds are asserted on both sides (exactly at the threshold and one above).
- A seed returning a keyed structure of integration-status variants feeds the shared helper that derives
  each expected indicator.
- A presence/absence feature uses TWO seeds (one without, one with). Creation fills the drawer from a
  client-side model factory.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../../../../pageObjects/pages/Group/subunits/`](../../../../../../pageObjects/pages/Group/subunits/CLAUDE.md) | drawer [`../../../../../../pageObjects/components/drawers/group/subunits/`](../../../../../../pageObjects/components/drawers/group/subunits/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
