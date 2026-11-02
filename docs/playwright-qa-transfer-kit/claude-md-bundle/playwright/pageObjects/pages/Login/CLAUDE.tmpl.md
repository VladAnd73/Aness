# pages/Login - The public auth screens

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **login.ts** - `Login`, the sign-in page (`url` = `/login`). Registered as the `loginPage` fixture.
  `validateVisible()`, `validateLegalFooter()`, `validateSsoButtons()`, `fillCredentials()`,
  `enterEmail()` / `enterPassword()` / `submit()` (for the form-validation cases),
  `signIn(user)` (-> `Dashboard`), one `signInWith<Provider>()` per SSO provider,
  `signInRemembering(user, remember)` - which also asserts what the sign-in request carried -
  `validateRememberMe(checked)`, `validateSignedIn()`, `validateCredentialsRejected()`,
  the field-error reads (`validateEmailError`, `validatePasswordRequired`,
  `validateEmailErrorNotShown`, `validatePasswordErrorNotShown`), and `startForgotPassword()`
  (-> `ForgotPassword`).
- **forgotPassword.ts** - `ForgotPassword` (`url` = `/forgot-password`). `validateVisible()` (which also
  asserts the URL), `enterEmail()` / `blurEmail()`, `requestReset(email)` (fills and waits for the
  request), the outcome reads (`validateInvalidEmailError`, `validateUnknownUserError`,
  `validateResetSent`), and `goToLogin()` / `backToLogin()`.
- **resetPassword.ts** - `ResetPassword`, reached by `urlForToken(token)` with a token a seed returned.
  `validateVisible()`, the field entries and blurs, the requirements notice
  (`validateRequirementsShown` / `NotShown`), `validateStrengthLabel(strength)`,
  `validateConditionsError()`, `validatePasswordsMismatchError()`, `validateSubmitEnabled(enabled)`,
  `setNewPassword(password)` (fills both fields, submits, waits for the request),
  `validateResetComplete()` / `validateResetExpired()`, and `goToLogin()` / `backToLogin()`.

## Conventions & gotchas
- `login.ts` is the real flow - distinct from the minimal validation-only stub at
  [`../Login.ts`](../CLAUDE.md). Use `loginPage` for credential/SSO sign-in specs. **The global auto
  fixture already logs in as an admin for every test, so an auth spec must first
  `userFlows.clearSession({ cookies: true })`** - a leftover token makes the login page redirect away.
- **`goToLogin()` / `backToLogin()` return nothing on purpose.** Returning a `Login` here would make
  these three files import each other in a cycle; the spec destructures the `loginPage` fixture and
  asserts on that instead.
- `validateSignedIn()` asserts the dashboard URL and that the form is gone - not dashboard content,
  which depends on the user's tenant and is a different scenario.
- `ResetPassword` and `ForgotPassword` share the page-level `form-section-*` and `notification-*` test
  ids with `Login`; each is only ever used on its own route, so the getters stay unambiguous. Keep it
  that way rather than widening a getter across routes.
- `signInRemembering` asserts the flag on the **outbound sign-in request**, not the resulting
  remember-me cookie. The cookie IS stored under this harness - but the harness launches
  Chrome with `--disable-web-security` while the app's sign-in is a cross-origin call that sends no
  credentials, so asserting the cookie would assert the harness rather than the app. Remember me has no
  rendered effect to assert instead, and the app's own session is a `localStorage` token
  (`{{TOKEN_STORAGE_KEY}}`) that outlives the browser either way.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../tests/auth/CLAUDE.md`](../../../tests/auth/CLAUDE.md) and [`../../../tests/main/login/CLAUDE.md`](../../../tests/main/login/CLAUDE.md) | auth flows in [`../../../support/`](../../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
