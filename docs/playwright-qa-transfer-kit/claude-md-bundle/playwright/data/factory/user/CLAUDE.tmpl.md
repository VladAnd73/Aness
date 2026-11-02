# data/factory/user - User form-input builder

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **user.ts** - `createDefaultUser(overwrites?)` -> a `User` (`~/cypress/common/types/userTypes`) whose
  name / title / email / phone number are what a spec types into the add-edit-user drawer.

## Conventions & gotchas
- The phone number is generated in the app's default phone region, because the drawer's phone field
  validates the number and the user pages format it with that region.
- Emails are lower-cased: the app stores and renders them lower-cased, so an upper-cased value would
  make a row assertion fail on a difference the user never sees.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | drawer [`../../../pageObjects/components/drawers/user/CLAUDE.md`](../../../pageObjects/components/drawers/user/CLAUDE.md) | specs [`../../../tests/account/users/CLAUDE.md`](../../../tests/account/users/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
