# playwright/data/factory/frozenDomain/group/subunit - Subunit builders

<one line: what this builder produces>. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **<file>.ts** - <the `createDefaultX(overwrites?)` / `createXList(count)` pair, the field groups it fills,
  and where its enum values (type/status/ownership) come from>

## How to use it
<which spec(s) use it and which drawer page object it fills; how to override the asserted fields>

## Conventions & gotchas
- Fields that are **computed, not random** (e.g. a total that is the sum of its parts) stay consistent
  only when overridden together - override deliberately.
- Relationship ids default `undefined`; set them (and seed the matching related rows) if the test needs
  the relationship.
- <whether this builder nests other factories>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | <where the shared types live>.

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
