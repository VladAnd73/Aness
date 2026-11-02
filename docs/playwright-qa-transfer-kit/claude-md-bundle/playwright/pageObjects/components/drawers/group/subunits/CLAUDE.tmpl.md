# drawers/group/subunits - Subunit & document drawers

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **editSubunits.ts** - `EditSubunitDrawer`. <the large create/edit/duplicate form: mode-aware
  `validateVisible()`, the detail fill, save (and the page it returns), composed sections, warning/tip
  helpers; fixture name>
- **downloadDocs.ts** - `DownloadDocsDrawer`. <document download from {{VENDOR_B}}: validators,
  select/download/search methods; fixture name>. Internally reuses `ListTables` **scoped to the drawer**
  for row logic.

## How to use it
<which list-page methods open each drawer; which factory builds the form input>

## Conventions & gotchas
- Some getters mix `getByTestId` with id-based selectors and `.nth()` indexing for compound numeric
  inputs - read the getter before reusing.
- `DownloadDocsDrawer` demonstrates the **table-in-a-drawer** pattern: `new ListTables(page, drawer)`
  scopes all row helpers to the drawer locator. Follow it rather than duplicating table logic.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | tables [`../../../sections/list/`](../../../sections/list/CLAUDE.md) | specs [`../../../../../tests/main/frozenDomain/group/subunits/list/`](../../../../../tests/main/frozenDomain/group/subunits/list/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
