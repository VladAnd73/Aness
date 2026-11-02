# tests/main/frozenDomain/sidebar - Frozen-domain navigation & language

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
| <spec file> | <navigate to each area via the sidebar; logo destination> | <seed path> | <yes/no> |
| <spec file> | <switch to each supported language; persists across reload> | <seed path> | <yes/no> |

<POs used.>

## Patterns & gotchas
- <logo destination for this area>. The language spec loops the languages and reloads to confirm the
  change applied to all elements.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | (main-area nav is in [`../../sidebar/`](../../sidebar/CLAUDE.md)).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
