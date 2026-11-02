# drawers/media - Media library drawer

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **mediaLibraryDrawer.ts** - `MediaLibraryDrawer`. <upload/search/select/delete methods; which upload
  mutations `uploadMedia()` waits for; exists / not-exists validators>

## How to use it
<which flows open it>. Uploads use `setInputFiles` with fixtures from
[`../../../../data/fixtures/files/`](../../../../data/fixtures/files/CLAUDE.md).

## Conventions & gotchas
- Handles both empty-library and populated states via conditional locators - a component with an empty
  state needs both branches in the page object.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
