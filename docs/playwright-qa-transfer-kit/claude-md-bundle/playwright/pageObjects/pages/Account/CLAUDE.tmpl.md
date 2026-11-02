# pages/Account - The `/account/**` screens

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **users.ts** - `Users` (route `/account/users`), the user list. Registered as the `usersPage`
  fixture. Holds a `ListTables` and delegates every row operation to it. `url`,
  `urlForSearch(phrase)` (the list is URL-driven, so a spec can start already
  filtered), `validateVisible()`, `validateZeroState()`, `validateUsersList(users)`,
  `validateUserRow(user)`, `validateUserStatus(user, status)`, `validateUserNotListed(user)`,
  `verifyListedUsersCount(n)`, pagination (`validatePaginationDoesntExist`, `goToNextPage`,
  `goToPreviousPage`), search (`searchForUser`, `clearSearchWithButton`, `clearSearchField`,
  `validateNoSearchResults`), and the row actions `openUser` / `openUserDetailsViaActionsMenu` (->
  `UserDetails`), `openEditUserDrawer` (-> `AddEditUserDrawer`), `removeUser`, `deactivateUser`,
  `activateUser`, `verifyUserActionNotAvailable`.
- **userDetails.ts** - `UserDetails` (route `/account/users/:uuid`), the single-user page. Instantiated
  inline (`new UserDetails(page)`) or returned by a `Users` navigation method. `urlFor(user)`,
  `validateVisible(user)`, `validateDetails(user)`, `validateStatus(status)`, `toggleStatus()`,
  `validateMissingProfilePicture()` / `validateProfilePicture()`, `backToUsers()`, `openEditDrawer()`
  and `startAddingProfilePicture()` (both -> `AddEditUserDrawer`).

## Conventions & gotchas
- Row lookup goes through `ListTables`' by-name column helpers, so a `User` from a seed is enough to
  address its row - no uuid plumbing.
- **The phone number and the created date are rendered formatted, so neither is compared literally.**
  The phone is matched with `phoneDigitsPattern` (the digits, in order, whatever the formatting) and the
  date with `formatDateLikeBrowser` (evaluated in the page, so the browser's own locale decides) - both
  in [`../../../support/utils.ts`](../../../support/CLAUDE.md).
- `validateDetails` deliberately leaves the "Updated" value alone: two blocks on that page share one
  test id, so the locator is ambiguous. Assert it only once the app gives them distinct ids.
- `validateDetails` expects the **User role** value to be the fallback the page shows when the user has
  no tenant role. A spec for a user who *does* have one needs that expectation derived from the seed's
  role - extend the method with an optional expected role rather than letting a tenant user assert the
  fallback and pass for the wrong reason.
- The status toggle is the **checkbox inside `toggle-content`**, not the `role="switch"` label beside
  it: the checkbox is absolutely positioned over that label and intercepts the pointer, so a click on
  the switch never lands.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | table machinery [`../../components/sections/list/CLAUDE.md`](../../components/sections/list/CLAUDE.md) | drawer [`../../components/drawers/user/CLAUDE.md`](../../components/drawers/user/CLAUDE.md) | specs [`../../../tests/account/users/CLAUDE.md`](../../../tests/account/users/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
