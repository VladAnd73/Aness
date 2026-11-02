# playwright/data/factory/member - Main-area data builders

Faker builders for the main-area domain. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| File | Builds |
|------|--------|
| `<entity>.ts` | <one row per builder file: the entity it builds, its key fields, and which nested builders or preset arrays it draws from> |

## How to use it
<how a spec calls the builders (`createDefaultX()` / `createXList(n)`, overwrites for asserted fields) and
which create-drawer page object method typically consumes the result, e.g.
`createMemberDrawer.fillOutMemberAndSubmit(member)`>

## Conventions & gotchas
- Builders that build **nested** entities (e.g. an item with a nested record) are controlled via
  overwrites - don't depend on the random nested defaults.
- A builder whose valid values depend on a per-destination constant (e.g. destination-specific option
  lists) imports that constant from `models/general` rather than hard-coding it; a binary asset path
  (a video field) points at the single file under `data/fixtures/files/`.
- <where the types come from: `models/member/*` and `~/cypress/common/types/*`>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | types in [`../../models/member/`](../../models/member/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
