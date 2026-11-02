# modals/orders - Change-bundle modal

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **changeBundleModal.ts** - `ChangeBundleModal`. <the multi-state change flow (choose -> confirm ->
  success/error): per-state validators, select/confirm/cancel/close, the full-flow helper, retry>

## How to use it
<which page method returns it>. It is a multi-state modal - methods assume correct state progression.

## Conventions & gotchas
- Imports a type from an app page's `types` module (type-only; NOT the `~/utils` barrel).

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../../tests/main/orderManagement/`](../../../../tests/main/orderManagement/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
