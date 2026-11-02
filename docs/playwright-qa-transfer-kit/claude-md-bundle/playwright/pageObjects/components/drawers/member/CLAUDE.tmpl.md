# drawers/member - Create-member drawer

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **createMember.ts** - `CreateMemberDrawer`. <the form it models and its key methods: `validateVisible()`,
  the fill-and-submit method (which mutation it waits for and which page it returns), required-field
  error checks>

## How to use it
<which page/sidebar action returns it, and which factory builds its input>

## Conventions & gotchas
- Masked inputs (e.g. phone) use `pressSequentially`, not `fill`.
- Media is attached via `generateMediaFilesForUpload` + `setInputFiles`.
- <fixture vs inline instantiation>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | factory [`../../../../data/factory/member/`](../../../../data/factory/member/CLAUDE.md) | specs [`../../../../tests/main/member/`](../../../../tests/main/member/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
