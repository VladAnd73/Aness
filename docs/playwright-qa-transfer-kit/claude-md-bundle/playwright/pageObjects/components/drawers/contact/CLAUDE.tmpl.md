# drawers/contact - Contact drawer (create + edit)

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **contactDrawer.ts** - `ContactDrawer`. <the one form the app renders for BOTH create and edit: its
  visibility validators per mode, the fill-and-submit methods (and which page each returns), the edit
  method, the generic submit method, required-field checks>
- **<import>.ts** - <the CSV import flow drawer>

## How to use it
<which page actions open it; which factory builds its input; which fields are optional>

## Conventions & gotchas
- When one drawer serves create AND edit with the same test id, use a visible difference (e.g. the
  Save vs Create button label) as the mode discriminator in `validateVisible`.
- A generic submit helper that waits for a mutation must be told which one on the edit path (e.g.
  `'mutation updateContact'`), otherwise it waits for the create mutation and times out.
- <the distinct submit paths by context; inline instantiation>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | specs [`../../../../tests/main/contacts/`](../../../../tests/main/contacts/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
