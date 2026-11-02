# tests/main/adminPanel/vendorContracts - Vendor contracts

The admin surface for vendor contracts and its effect on the {{VENDOR_A}} payload.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <admin spec: the contract catalog CRUD + search, and the tenant-to-contract link CRUD incl. rejected links,
  each asserted on the link list AND on every affected tenant page>
- <payload spec: the contract ids each payload carries, asserted at the {{MOCK_TOOL}} transaction log after the
  real per-destination republish button, across inheritance cases and after admin edits; the flag variants>

## How to use it
- <the shared seed, its returned shape (a tenant tree with own/inherited/overriding contracts plus a tenant
  with none), and its flag option; an unknown option value makes the seed raise>
- Assert the payload with a `vendorAMock` verifier after a purge; a spec-local helper does
  purge -> republish -> verify, so one test can republish the same item before and after a change.
- Pick a tenant or a contract in a link form by its id, never by the dropdown's display text.

## Conventions & gotchas
- Needs {{MOCK_TOOL}} on `{{VENDOR_A_MOCK_PORT}}` and the backend booted with `{{REVIEW_APP_FLAG}}`.
- Slow tests (many republishes each waiting out a settle window) are full regression only, not `@smoke`.
- Search terms come from the seed's returned data (a shared id prefix, a label), never literals.
- **Each `test.fixme` here is a filed bug whose body is the intended behaviour**; it turns green when the
  bug is fixed. Each carries its ticket as an `issue` annotation (shown in the report); when a ticket is
  fixed, drop `.fixme` and keep the annotation as the link to that fix. Keep a table here of
  spec | fixme title | ticket.
- A behaviour with no UI to assert has no test; say so here rather than inventing a backend read.
- An open product question gets no test until it is answered.
- On a just-booted backend the first seed can take most of the local timeout budget; warm the backend
  with one seed before judging a first-test timeout.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects
[`../../../../pageObjects/pages/AdminPanel/`](../../../../pageObjects/pages/AdminPanel/CLAUDE.md) | mock helper
[`../../../../support/CLAUDE.md`](../../../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
