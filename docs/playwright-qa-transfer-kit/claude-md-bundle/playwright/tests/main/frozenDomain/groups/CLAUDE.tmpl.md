# tests/main/frozenDomain/groups - Group list & details specs

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
| <spec file> | <one row per spec: row actions in both views, create modal, open from list/search, details statistics, integration-status variants, search, pagination> | <seed path (count)> | <yes/no> |

<POs used.>

## Patterns & gotchas
- List specs iterate `['Grid','List']` and switch via `switchToView`. <search minimum length>
- A seed that returns a keyed structure of variants (one entry per integration state) drives a nested
  `describe` with per-variant assertions - the canonical one-seed-many-variants example the `new-test`
  skill cites.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page object [`../../../../pageObjects/pages/Group/`](../../../../pageObjects/pages/Group/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
