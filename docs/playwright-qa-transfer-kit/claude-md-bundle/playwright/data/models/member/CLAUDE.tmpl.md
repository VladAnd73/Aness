# playwright/data/models/member - Main-area types & presets

Type aliases and preset constant arrays for the main-area domain. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| File | Provides |
|------|----------|
| `<entity>.ts` | <one row per file: the model type(s) and the preset arrays it exports; mark which file is the source of truth for a given kind of test data> |

## How to use it
<import a preset to pick a valid value or a type to annotate factory output / seed data; name the preset
array the member/record factories draw from>

## Conventions & gotchas
- A model type whose fields are nearly all optional (because the frontend uses different subsets per
  context) must be narrowed with overwrites in factories - don't assume a field is present.
- When a model pulls an enum from a page object or from `models/general`, keep those in sync if the
  underlying enum changes.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | used by [`../../factory/member/`](../../factory/member/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
