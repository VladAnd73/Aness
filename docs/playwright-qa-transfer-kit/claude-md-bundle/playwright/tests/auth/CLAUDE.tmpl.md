# tests/auth - Sign-in and password-reset specs

The public authentication routes: `/login`, `/forgot-password`, `/reset-password`.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **loginCredentials.spec.ts** - `/login`: email normalisation (upper-cased, whitespace-padded),
  Remember me on and off, rejection of a wrong password and of an unknown email, SSO, and the
  form's own required/invalid-email validation. Seeds `seeds/app/auth/user` (an email/password pair)
  and `seeds/app/users/sso_mock` for SSO.
- **passwordReset.spec.ts** - `/forgot-password` (request a reset, malformed and unknown addresses, the
  navigation both ways) and `/reset-password` (the requirements notice, the strength label, when the
  submit button unlocks, leaving without resetting, the full reset chain, and an expired link). Seeds
  `seeds/app/auth/{reset_password_token,expired_reset_password_token}`, which **return the token**, so
  a spec navigates straight to the reset URL.

## Conventions & gotchas
- **Every spec here starts with `userFlows.clearSession({ cookies: true })`.** The shared fixture logs
  in as an admin before every test; a leftover `{{TOKEN_STORAGE_KEY}}` makes the login page redirect away
  before the first assertion. The `cookies: true` part matters after a *successful* reset: the reset signs
  the user in server-side, and a surviving session cookie lets the next sign-in succeed without the
  credentials being checked - which would make the "old password no longer works" assertion pass for the
  wrong reason.
- A successful sign-in is asserted with `loginPage.validateSignedIn()` (the app is reached and the form
  is gone). Don't assert dashboard content here: these seeds' users belong to no tenant, so what the
  dashboard renders is a different scenario from the one under test.
- **The reset link is never read out of an email.** The seed returns the token, so nothing here depends
  on the backend's app-URL setting - which is what makes these specs behave the same locally as in CI.
- Passwords come from a shared test-data module; each is the minimal witness for its strength tier, which
  is what makes the strength-label assertions meaningful.
- **A `test.fixme` here can be a product defect, not a flake.** Keep the assertion as the intended
  behaviour, leave it `fixme` until the app is fixed, and never relax it to match the current behaviour.
- Credential/SSO coverage is split: the sign-in happy path and the SSO round-trip flows live in
  [`../main/login/`](../main/login/CLAUDE.md); everything that needs no tenant lives here. Only the
  *presence* of the SSO buttons is asserted here.
- **Before writing an SSO sign-in spec, read these two, in this order.**
  1. **It must sign in a user who belongs to a tenant.** A user with no tenant is not routed back to
     {{PROJECT_NAME}} by the OAuth callback - so the spec fails looking exactly like broken SSO. `main/login/login.spec.ts` seeds a tenant for this
     reason; reuse that setup rather than adding a tenant seed here.
  2. **The SSO round-trip is intermittently broken in dev mode, and no test-side change fixes it.**
     React `StrictMode` double-invokes the OAuth-callback page's effect, so the single-use temp token can
     be exchanged twice; the second exchange fails and the app replaces the URL with `/login`. Do not add
     another test to that family expecting it to be stable.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../pageObjects/pages/Login/CLAUDE.md`](../../pageObjects/pages/Login/CLAUDE.md) | auth helpers [`../../support/CLAUDE.md`](../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
