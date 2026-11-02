# pages/Orders - Order management

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <one line for the order-management page object: class name, route, the login-less entry point
  `openWithPreviewToken(uuid, token)` (builds the query with `URLSearchParams`, so both params are
  encoded), the activate / cancel / change-bundle actions and section validators, fixture name>

## Conventions & gotchas
- <which page sections are visible before vs after the order is activated>
- Open a token-gated page through the page object's opener rather than building the query in the
  spec. The uuid + token come from the seed's returned JSON (a preview token) - never minted via `eval`.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | modal [`../../components/modals/orders/`](../../components/modals/orders/CLAUDE.md) | specs [`../../../tests/main/orderManagement/`](../../../tests/main/orderManagement/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
