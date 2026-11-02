# tests/account/users - Tenant user management specs

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **userList.spec.ts** - `/account/users`: the list and its search (incl. the no-results and cleared
  states), remove, edit-from-the-list, opening a user by row click and from the actions menu, the same
  actions taken while a search filter is applied, pagination on and off, and the zero state. Seeds
  `seeds/app/users/multiple_users` (its `count` option drives the pagination cases).
- **userDetails.spec.ts** - `/account/users/:uuid`: the profile read (full, and the gaps of a user with
  no phone number or picture), the three status badges on both the list and the details page,
  activate/deactivate from the list and from the details toggle, editing personal details, and adding a
  profile picture. Seeds `seeds/app/users/users_statuses`, which returns one user per status - the
  three statuses in one seed.

## Conventions & gotchas
- **Do not `changeTenantTo` here.** The seeded users belong to no tenant, and the users query is
  tenant-scoped when a tenant is active - setting one empties the list.
- The signed-in admin the shared fixture creates is a super-admin and is **excluded** from this list, so
  a seeded count is the whole list, and a seed count equal to the page size fills exactly one page.
- Status is a derived value, so assert it through the badge (`validateUserStatus`) after the action, and
  assert the complement - a deactivated user must no longer offer "Deactivate".
- Editing a user goes through the shared add-edit-user drawer, which discards a value typed before its
  organization options resolve; the drawer page object re-types until the value sticks, so specs just
  call the drawer's fill-and-submit method.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../pageObjects/pages/Account/CLAUDE.md`](../../../pageObjects/pages/Account/CLAUDE.md) | drawer [`../../../pageObjects/components/drawers/user/CLAUDE.md`](../../../pageObjects/components/drawers/user/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
