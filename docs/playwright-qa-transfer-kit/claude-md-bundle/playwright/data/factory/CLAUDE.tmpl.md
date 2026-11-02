# playwright/data/factory - Faker data builders

Builder functions that generate client-side objects to fill forms. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **[`member/`](member/CLAUDE.md)** - main-area domain builders (one file per entity the main area's
  forms take).
- **[`frozenDomain/`](frozenDomain/CLAUDE.md)** - frozen-domain builders (group subunits).
- **[`user/`](user/CLAUDE.md)** - the user the add/edit user drawer is filled with.

## How to use it
Import the `createDefault*` / `create*List` function for the entity and pass overwrites for the fields
your assertion cares about:
```typescript
import { createDefaultMember } from '~/playwright/data/factory/member/member';
const member = createDefaultMember({ name: 'Fixed Name' });
```
The common shape per entity is `createDefaultX(overwrites?)` (one object) and `createXList(count)`
(a batch - each element independently randomized).

## Conventions & gotchas
- Uses `@faker-js/faker`, so each call is **non-deterministic** - always pin the fields you assert on
  via overwrites. Batch builders call the factory per element (all unique).
- Factories import types/preset arrays from `~/playwright/data/models/*` and random/file helpers from
  `~/playwright/support/utils` - never `~/utils`.
- Some factories nest others (an item always builds a nested record; a contact nests record +
  member) - override the nested piece rather than relying on its random default.
- These build **form input**, not backend rows. Data you need to already exist in the DB comes from a
  seed.

## Cross-links
Children: [`member/`](member/CLAUDE.md) | [`frozenDomain/`](frozenDomain/CLAUDE.md) | [`user/`](user/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
