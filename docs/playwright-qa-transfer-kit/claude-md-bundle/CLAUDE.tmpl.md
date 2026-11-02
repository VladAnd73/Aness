# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

<one paragraph: {{PROJECT_NAME}} is a <frontend stack> app that talks to {{BACKEND_REPO}} via <REST / GraphQL / subscriptions>.>

## Common Commands

<one fenced block grouped by purpose: dev server (port {{FRONTEND_PORT}}), unit tests (single run, watch, single file), lint/format/circular-deps, build, component explorer, e2e (Playwright run, headed run, legacy runner if any).>

## Architecture

### Tech Stack

<one bullet per framework/build tool/language/styling library with its major version.>

### Data Fetching

<one numbered entry per protocol (e.g. REST client, GraphQL client, server-state cache) with the file that configures it and the API base path on {{BACKEND_URL}}.>

### Authentication

<where the session token lives ({{TOKEN_STORAGE_KEY}}), impersonation key if any, SSO providers.>

### Key Directories

<an ASCII tree (`|--` / `` `-- ``) of `src/` with one comment per folder.>

### Provider Hierarchy

<the root provider chain as `A -> B -> C`.>

### Code Splitting

<how routes are lazily loaded.>

## Internationalization (i18n)

<library, master language, supported locales, the extract/upload/download commands, and the rule "use the library's components/macros for every user-facing string".>

## Local Development with the Backend

<two fenced commands: boot {{BACKEND_REPO}} on {{BACKEND_PORT}} and the frontend on {{FRONTEND_PORT}} with {{BACKEND_URL_ENV}} pointing at it.>

## Environment Variables

<one bullet per key variable from the example env file: {{BACKEND_URL_ENV}}, {{FRONTEND_ENV_VARS}}.>

## Styling / UI Components / Submodules

<class-ordering rule, shared UI library and what not to duplicate, submodule setup/update commands.>

---

## Frontend Best Practices

<summary of `.cursor/rules/bestpractices.mdc` (see editor-rules/bestpractices.tmpl.mdc).>

## Testing Rules

The content below summarizes `.cursor/rules/testing.mdc`.

**Unit tests are required** for new or modified utils and hooks. Add co-located `*.test.ts` or `*.test.tsx`. E2E tests (Playwright) are not required for every task.

**Before writing assertions**, read "What to Assert" in `.cursor/rules/testing.mdc`: every AC gets an assertion (including linked tickets' ACs), every named population gets its complement tested, and assert the rule rather than a value the test itself supplied.

**A test with no demonstrated pre-fix failure is superficial.** Run every regression assertion against code carrying the bug it guards and show it red before landing it. An assertion that sits behind an earlier one which aborts on that code never ran - split the test until each rule fails on its own. Deepen a shallow assert in the change that touches it; "pre-existing" is not a defence.

- Use `describe` blocks to group related tests.
- Use descriptive test names like `it('returns formatted price')` or `it('should return formatted price when value is valid')`; both styles are acceptable - be consistent within a file.
- Follow Arrange-Act-Assert; keep tests focused and independent.
- Use `@testing-library/react`, `@testing-library/user-event`, and `@testing-library/jest-dom` for component tests.
- Test custom hooks with the Template component pattern or `renderHook` when appropriate.
- Mock external dependencies and API calls (prefer MSW for network).
- For E2E tests:
  - Use Playwright with Page Object Model.
  - Keep tests isolated and independent.
  - Prefer selectors such as `getByRole` and `getByTestId`.

## Data-Fetching Library Rules

<summaries of any per-library editor rules (GraphQL client, server-state cache), one bullet per rule.>

## Documentation of New Patterns

The content below summarizes `.cursor/rules/documentation.mdc`.

When introducing or changing **how** something is done (new pattern, approach, architecture, or convention), update the relevant `.md` files:

- **CLAUDE.md** - Reflect changes in conventions or patterns for AI/developers.
- **.cursor/rules/\*.mdc** - Add or edit rules when you introduce a new convention.
- **README.md** - When setup, usage, or architecture changes.
- **docs/** - Feature docs, architecture docs, or technical specs.

### Playwright suite per-folder CLAUDE.md

Every folder under `{{E2E_ROOT}}` carries its own `CLAUDE.md` documenting what the folder holds and the
conventions that apply there. When you add, move, rename, or delete a file in a `{{E2E_ROOT}}` folder,
update that folder's `CLAUDE.md` in the same change; when you add a new folder, give it a `CLAUDE.md`
with the standard `> **Maintenance:**` footer. Start from [`playwright/CLAUDE.md`](playwright/CLAUDE.md).
