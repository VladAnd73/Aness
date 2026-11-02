# pages/Dashboard - Dashboard

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <one line for the dashboard page object: class name, route, card validators, filter-drawer openers
  (each returning its drawer object), fixture name>

## Conventions & gotchas
- Localized assertions: the page object holds a text map keyed by `Language`; pass the language so the
  expected strings match the rendered locale.
- <which area variant shows which cards>
- Role-gated cards are asserted through a `hasXVisible(visible)` method, both directions.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | filter drawers [`../../components/drawers/dashboard/`](../../components/drawers/dashboard/CLAUDE.md) | specs [`../../../tests/main/dashboard/`](../../../tests/main/dashboard/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
