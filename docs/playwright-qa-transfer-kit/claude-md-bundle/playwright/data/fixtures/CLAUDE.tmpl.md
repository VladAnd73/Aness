# playwright/data/fixtures - Static mock payloads & assets

Static, canned data: GraphQL-mock responses and a binary upload asset. No faker, no builders.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`Responses/`](Responses/CLAUDE.md)** - canned app-query responses used to mock GraphQL via
  `page.route()` (item restrictions, option lists, a lookup search).
- **[`common/`](common/CLAUDE.md)** - shared test constants (mutation-name lists, table view types).
- **[`files/`](files/CLAUDE.md)** - a binary MP4 used by file-upload / video tests.

## How to use it
Import a response object and fulfil a route with it inside a page object's `intercept*` helper (see
`pageObjects/BasePage.ts`). Files are referenced by path via `support/utils.ts` upload helpers.

## Conventions & gotchas
- These payloads are **hand-maintained snapshots** of the backend contract - if the backend response
  shape changes, a mocked spec can pass on stale data. Prefer asserting rendered UI outcomes over the
  mock's own values.

## Cross-links
Children: [`Responses/`](Responses/CLAUDE.md) | [`common/`](common/CLAUDE.md) | [`files/`](files/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
