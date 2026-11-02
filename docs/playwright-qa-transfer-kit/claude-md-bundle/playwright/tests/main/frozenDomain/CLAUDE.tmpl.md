# tests/main/frozenDomain - Frozen-domain specs

> **DISABLED - every `test.describe` in this folder is `test.describe.skip`.** This domain is out of
> scope going forward (the decision is recorded in {{TRACKER}}), so nothing under
> `playwright/tests/main/frozenDomain/**` executes in CI. **Do not un-skip a describe and do not add a
> spec here.** If a behaviour still matters, cover it under `playwright/tests/main/<domain>/`. The
> tables below remain the record of what this folder once exercised, not of what runs - treat every
> `@smoke` marking as historical.

<one line on what this area once covered>. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`groups/`](groups/CLAUDE.md)** - group list CRUD, search, pagination, details, integration status.
- **[`group/`](group/CLAUDE.md)** - single-group sub-features -> **[`group/subunits/`](group/subunits/CLAUDE.md)**.
- **[`items/`](items/CLAUDE.md)** - group-scoped item list actions.
- **[`sidebar/`](sidebar/CLAUDE.md)** - frozen-domain sidebar navigation + language switch.
- **[`topNavbar/`](topNavbar/CLAUDE.md)** - the "Create new" flows.

## Patterns common to this area
- <where this area's seeds live and the usual `beforeEach` route>
- List specs loop over `['Grid', 'List']` view variants and reuse the shared table page objects.
- Integration-status expectations (banners, indicator colors, source/sync badges) are derived from the
  seed by one shared support helper.
- <which specs touch {{VENDOR_B}}>

## Cross-links
Children listed above. Parent [`../CLAUDE.md`](../CLAUDE.md) | group page objects [`../../../pageObjects/pages/Group/`](../../../pageObjects/pages/Group/CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
