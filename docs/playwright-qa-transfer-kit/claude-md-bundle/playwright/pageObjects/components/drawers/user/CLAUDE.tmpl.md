# drawers/user - The add/edit user drawer

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **addEditUser.ts** - `AddEditUserDrawer`, the drawer that both `/account/users` (list row -> Edit) and
  `/account/users/:uuid` (details -> Edit / Add image) open. `validateVisible(mode)` /
  `validateNotVisible()`, the field entries (`enterName`, `enterTitle`, `enterEmail`,
  `enterPhoneNumber`), `uploadProfilePicture()`, `submit()`, and `fillOutUserAndSubmit(user)` - pass a
  `createDefaultUser()` from [`../../../../data/factory/user/`](../../../../data/factory/user/CLAUDE.md).
  Add getters for field-level validation together with the spec that asserts them, not before.

## Conventions & gotchas
- **Field entry re-types until the value is retained**, because the drawer re-initialises its form once
  its async options arrive and drops whatever was typed first. That is why there is no plain
  `fill` here and why specs must not reach past this object to the inputs.
- `submit()` waits for the `updateUser` mutation and then for the drawer to unmount, so a spec can
  assert the resulting row or page immediately.
- The picture flow uploads through the drawer's own file input and dismisses the crop dialog; the file
  comes from `generateMediaFilesForUpload`, so no new asset is needed.
- The submit button is disabled while the form is pristine - a picture-only change still counts as
  dirty, an unchanged form does not.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | pages [`../../../pages/Account/CLAUDE.md`](../../../pages/Account/CLAUDE.md) | factory [`../../../../data/factory/user/CLAUDE.md`](../../../../data/factory/user/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
