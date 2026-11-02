# tests/account - The `/account/**` specs

Specs for the account area. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`users/`](users/CLAUDE.md)** - tenant user management (`/account/users`, `/account/users/:uuid`).

## Why this is a top-level folder, not a `main/` domain
`/account/*` is rendered **outside** the role-gated route blocks, so every persona reaches it
- whichever role they hold - and the user-menu entries that lead here carry no role
gate. A spec here must therefore not assume a particular tenant context: do not `changeTenantTo` unless
the scenario is specifically about a tenant-scoped view. Tenant-scoping the session narrows what the
user list returns, which is a different scenario from the ungated one.

## Where to add a spec
Account screens with no specs yet each get their own subfolder (with its own `CLAUDE.md`) rather than
widening `users/`.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../pageObjects/pages/Account/CLAUDE.md`](../../pageObjects/pages/Account/CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
