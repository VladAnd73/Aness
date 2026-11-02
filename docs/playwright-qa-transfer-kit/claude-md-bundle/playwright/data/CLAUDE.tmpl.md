# playwright/data - Client-side test data

Client-side data the specs use to **fill forms** and **mock GraphQL responses** - this is NOT backend
seed data (that lives in {{BACKEND_REPO}}). Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here (three distinct kinds - don't mix them up)
- **[`factory/`](factory/CLAUDE.md)** - faker-based **builder functions** that generate objects a test
  types into the UI (e.g. a member to fill the create-member form). Non-deterministic; controlled via
  an `overwrites` argument.
- **[`models/`](models/CLAUDE.md)** - pure **TypeScript types** + preset **constant arrays** (enums of
  valid values). No logic, no faker.
- **[`fixtures/`](fixtures/CLAUDE.md)** - **static** canned payloads used to mock the app's own GraphQL
  queries via `page.route()`, plus a binary media file for upload tests.

## How to use it
- Need form input? `import { createDefaultX } from '~/playwright/data/factory/...'` and pass overwrites
  for the fields the assertion depends on.
- Need a valid enum value / a type? Import from `models/`.
- Mocking an app query in a page object? Pull the canned response from `fixtures/Responses/`.

## Conventions & gotchas
- **Factory != seed.** Factories never touch the backend; they build objects the UI consumes. Persistent
  DB state comes from backend seeds (`server.seed` via {{SEED_ENDPOINT}}). A test typically uses both,
  independently.
- Factories import from `models/` (types/consts) and `~/playwright/support/utils` (random/file helpers)
  - never from `~/utils` (the app's util barrel).
- Shared entity types come from `~/cypress/common/types/*`; `models/` adds test-specific type aliases
  and preset arrays on top.

## Cross-links
Children: [`factory/`](factory/CLAUDE.md) | [`models/`](models/CLAUDE.md) | [`fixtures/`](fixtures/CLAUDE.md). Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add or remove a subfolder here, update this CLAUDE.md in the same change.
