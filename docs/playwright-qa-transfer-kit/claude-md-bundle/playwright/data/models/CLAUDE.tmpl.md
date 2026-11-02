# playwright/data/models - Types & preset constants

Pure TypeScript types and preset constant arrays (valid-value enums). No logic, no faker.
Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`member/`](member/CLAUDE.md)** - main-area domain types + preset arrays.
- **[`general/`](general/CLAUDE.md)** - platform-wide destination identifiers shared across domains.
- **[`user/`](user/CLAUDE.md)** - user statuses, the user context-menu actions, and one password per
  strength tier.

## How to use it
Import a type or a preset array to build well-typed data or pick a valid value:
```typescript
import { itemDuration } from '~/playwright/data/models/member/item';
```
Consumed by `factory/` (for types + valid values) and by page objects (to validate against valid sets).

## Conventions & gotchas
- Pattern for enumerations: `export const x = [...] as const; export type X = (typeof x)[number];`
- Models import shared entity types from `~/cypress/common/types/*` but **never** import factories or
  `support/`. Keep them dependency-light so they stay importable everywhere.

## Cross-links
Children: [`member/`](member/CLAUDE.md) | [`general/`](general/CLAUDE.md) | [`user/`](user/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
