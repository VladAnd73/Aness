# tests/main/adminPanel - Admin panel specs

Server-rendered admin-panel specs (Rails Administrate, Turbo). Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`failedItems/`](failedItems/CLAUDE.md)** - the failed-items repair dashboard.
- **[`ordersOverview/`](ordersOverview/CLAUDE.md)** - order intake observability: list + drill-down.
- **[`autoOrderAttempts/`](autoOrderAttempts/CLAUDE.md)** - the automatic-order pipeline observability
  panel: list + drill-down + a related write-back show page.
- **[`vendorContracts/`](vendorContracts/CLAUDE.md)** - vendor contracts: the contract catalog, tenant-to-contract links, and
  the contract ids the {{VENDOR_A}} payload carries.
- <one line per spec file that sits directly in this folder, e.g. a global-nav link that redirects to an
  external tool with a signed single-sign-on token>

## How admin-panel specs differ from the rest of the suite
- Sign in with `userFlows.signInAsAdmin()` (NOT `changeTenantTo`); open pages via the admin page
  objects' `open`/`gotoAdmin`.
- Pages are on the **backend origin** (`{{BACKEND_PORT}}`), driven by **Turbo**, extending `AdminPanelBase`.
- **The backend must be booted with `{{REVIEW_APP_FLAG}}`**, which gates the passwordless test-only
  admin sign-in route.
- No frontend dev server is needed for a spec that only touches the admin panel.

## Cross-links
Children: [`failedItems/`](failedItems/CLAUDE.md) | [`ordersOverview/`](ordersOverview/CLAUDE.md) |
[`autoOrderAttempts/`](autoOrderAttempts/CLAUDE.md) | [`vendorContracts/`](vendorContracts/CLAUDE.md).
Parent [`../CLAUDE.md`](../CLAUDE.md) | page objects [`../../../pageObjects/pages/AdminPanel/`](../../../pageObjects/pages/AdminPanel/CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
