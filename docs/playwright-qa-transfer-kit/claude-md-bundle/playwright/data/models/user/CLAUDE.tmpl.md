# data/models/user - User statuses, actions and password presets

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **user.ts** - `userActions` / `UserAction` (the user row's context-menu options, folded into the
  shared `ContextMenuOptions` union) and `userStatuses` / `UserStatus` (the badge labels
  `Active | Deactivated | Invite sent`).
- **password.ts** - `passwordStrengths` (`weak | medium | strong`) + `PasswordStrength`, and
  `PASSWORDS`, one password per strength tier plus an `invalid` one.

## Conventions & gotchas
- Each `PASSWORDS` entry is the **minimal witness for its tier**: it satisfies that tier's requirements
  and fails the next tier's. `invalid` fails the base requirements. Keep that invariant when editing -
  a spec asserting the strength label depends on `medium` NOT qualifying as `strong`, and on `weak` NOT
  qualifying as `medium`. The requirements live in the app's password validators (in the reference
  project, `src/utils/validators.ts`); mirror a change to them here.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../tests/auth/`](../../../tests/auth/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
