# Playwright QA transfer kit

A kit for standing up a production-grade Playwright end-to-end (e2e) QA practice on a new project with a spore coordinator and its workers. It was distilled from a suite a QA team runs in production on a private web product (a React single-page app over a Rails API, with HTTP mocks for third parties), and sanitized for publication: the product, its domain, its vendors and its infrastructure are replaced by neutral names and `{{PLACEHOLDERS}}`.

The kit carries four things:
- **a transfer prompt** that a coordinator executes phase by phase;
- **a reference walkthrough** that explains the mechanism and reasoning behind each convention;
- **a `CLAUDE.md` bundle**: every per-folder agent-instructions file of the reference suite, templated;
- **the QA agent skills and one reviewer agent**, generalized.

## Layout

```
playwright-qa-transfer-kit/
|-- README.md                    this file
|-- 01-transfer-prompt.md        the executable prompt: (A) replicate as-is, (B) re-derive, (C) phases 0-5
|-- 02-reference-walkthrough.md  the reference suite, layer by layer: config, folders, seeding, CI, docs, tooling, couplings
|-- 03-testing-lessons.md        standalone generic testing lessons (assertions, isolation, CI forensics, scope)
|-- claude-md-bundle/
|   |-- MANIFEST.md              every bundle file with its tier and placeholders
|   |-- CLAUDE.tmpl.md           repo-root agent file (testing sections + outline)
|   |-- editor-rules/            always-applied editor rule files that touch tests (*.tmpl.mdc)
|   `-- playwright/              one CLAUDE.tmpl.md per folder of the e2e suite, mirroring its tree
|-- skills/                      generalized QA skills, one folder per skill (SKILL.md + references)
`-- agents/                      the report-only coverage reviewer agent
```

## How to use it

Use the pieces in this order. The prompt names the phase at which each other piece is supplied.

1. **Phase 0 (discovery).** Paste everything below the line in `01-transfer-prompt.md` into the receiving coordinator. It resolves every `{{PLACEHOLDER}}` (see the glossary below) into a `discovery.md` with a source for each value, and asks the operator only the decisions the prompt lists.
2. **Phases 1-3 (scaffold, first seeded spec, minimal CI).** The prompt is self-sufficient. Give workers `03-testing-lessons.md` as standing context, and `02-reference-walkthrough.md` when they need the reason behind a rule or the shape of a mechanism (the seed endpoint, the per-test fixture, the mock-log verifiers, the CI base workflow).
3. **Phase 4 (docs layer).** Supply `claude-md-bundle/`. Read `claude-md-bundle/MANIFEST.md` first:
   - **generic** files can be copied after resolving placeholders;
   - **mixed** files keep their conventions; replace the reference's domain lines with the new project's;
   - **project-specific** files are structural outlines only: headings plus `<angle-bracket>` notes saying what content goes there.
   Rename each `CLAUDE.tmpl.md` to `CLAUDE.md` and each `*.tmpl.mdc` to `*.mdc` when you install it. Links inside the bundle already point at the installed names, so they resolve only after the rename. Folder names follow the reference's tree with its domain folders renamed (`main/`, `item/`, `member/`, `record/`, ...): map them onto the new project's own domains.
4. **Phase 5 (agent skills).** Supply `skills/` and `agents/`. Bind each skill's placeholders to the Phase 0 answers and install them in the runtime's skill directory (for Claude Code, `.claude/skills/<name>/SKILL.md` and `.claude/agents/<name>.md`). Keep each skill's workflow and numbered rules; re-bind only the mechanics.

Run the prompt's done-checks at the end of every phase. After installing, `grep -rnE '\{\{[A-Z0-9_]+\}\}' <installed paths>` must print nothing. (Lowercase `{{...}}` in the seeds-and-mocks authoring rules is mock-template syntax, not a placeholder.)

### Skills in this kit

| Skill | Use it for |
|---|---|
| `new-test` | Scaffolding or reviewing one spec: intake, investigate before scaffold, the numbered authoring rules, the superficial-test hard rule. |
| `running-e2e-tests` | Running the suite locally with CI's frontend env, one run per port, readiness polling. |
| `starting-the-e2e-backend` | Booting one isolated backend per consumer in test mode; the seed-endpoint failure taxonomy; teardown. |
| `investigating-e2e-failures` | Triage from report and trace: localize the layer, classify, repeat to separate flaky from deterministic. |
| `reviewing-e2e-coverage` + agent `senior-qa-e2e-reviewer` | A report-only senior-QA audit of one feature's specs with the [Presence-only] / [Shallow] / [Deep] rubric. |
| `qa-e2e-test-only-changes` | The policy that QA fixes stay in test code and app fixes are documented for developers. |
| `testing-a-feature-end-to-end` | One worker taking a feature from ticket to reviewed PR and filed bugs, with independent review and a completeness check. |
| `analyzing-test-tickets` | Turning a ticket or chat thread into a Feature Analysis Report with suggested scenarios. |
| `editing-backend-seeds-and-mocks` | The declare-then-ack governance for any edit outside your own worktree, plus the seed and mock authoring rules (dynamic mock data, routes from the vendor's own API docs). |
| `sweeping-releases-for-e2e-coverage-gaps` | Finding shipped work without e2e coverage from the git diff between release tags. |


## The neutral vocabulary

The reference's product domain is replaced consistently across the kit:

| Kit word | Role in the reference |
|---|---|
| item | the main entity users create and that moves through a lifecycle (draft, live, paused, finished, archived) |
| bundle | a purchasable package that groups items |
| tenant | the organisation a user belongs to and switches between |
| member | a person in a tenant whom items belong to |
| record | a domain record an item is about |
| destination | an external target an item is distributed to |
| `{{VENDOR_A}}` | a third-party distribution API the backend pushes item updates to |
| `{{VENDOR_B}}` | an upstream system-of-record vendor feeding an async integration pipeline |
| main area (`main/`) | the main authenticated area of the app |
| frozen domain (`frozenDomain/`) | a product area whose specs are all skipped on purpose |
| report app (`ReportApp/`) | a separate public-facing app with its own production smoke |

## Placeholder glossary

Resolve every placeholder in Phase 0. A value with no counterpart on the new project is recorded in `discovery.md` as "not applicable", and the text that uses it is dropped.

**Project and repos**

| Placeholder | Meaning |
|---|---|
| `{{PROJECT_NAME}}` | The product under test. |
| `{{FRONTEND_REPO}}` / `{{BACKEND_REPO}}` | The frontend and backend repositories. |
| `{{MOCKS_HOME}}` | The one canonical home of mock-server environments (a repo or a folder). |
| `{{E2E_ROOT}}` | The e2e suite folder, written with a trailing slash (the reference used `playwright/` at the frontend root). |
| `{{E2E_REPO_LOCATION}}` | Whether the suite lives in the frontend repo or its own repo. |
| `{{DEFAULT_INTEGRATION_BRANCH}}` | The branch PRs target. |
| `{{RELEASE_BRANCH_PATTERN}}` / `{{RELEASE_TAG_PATTERN}}` | Release branch glob; release tag scheme. |
| `{{FRONTEND_CHECKOUT}}` / `{{BACKEND_CHECKOUT}}` | Local checkout paths of the two repos. |
| `{{FRONTEND_STACK}}` / `{{BACKEND_STACK}}` / `{{APP_API_STYLE}}` | Framework and dev server; backend language and framework; REST or GraphQL. |
| `{{FORMS_LIBRARY}}` / `{{I18N_LIBRARY}}` | The frontend's form library and its translation library. |
| `{{UI_LIBRARY_PORTALS}}` | UI-library elements rendered in portals (drawers, menus). |
| `{{ROLES}}` / `{{TENANCY_MODEL}}` | User roles; how tenants are modelled and switched. |
| `{{FIRST_JOURNEYS}}` | The first three user journeys to cover. |

**Runtime and URLs**

| Placeholder | Meaning |
|---|---|
| `{{FRONTEND_URL}}` / `{{FRONTEND_PORT}}` | Where the dev server listens. |
| `{{BACKEND_URL}}` / `{{BACKEND_PORT}}` | Where the test-mode backend listens. |
| `{{BACKEND_URL_ENV}}` | The env var the frontend reads for the backend URL. |
| `{{BACKEND_FRONTEND_URL_ENV}}` | The env var the backend reads for the frontend URL (redirects, SSO callbacks). |
| `{{FRONTEND_ENV_VARS}}` | Every env var the dev server needs, identical in CI and locally. |
| `{{MAP_TOKEN_ENV}}` | A frontend third-party token env var whose absence blanks a page. |
| `{{HOST_OS}}` / `{{DEVSHELL}}` | Host OS; the pinned dev shell that provides a chromium matching the Playwright version. |
| `{{SECRETS_WRAPPER}}` | The command that injects secrets into a process. |
| `{{SERVICES_WRAPPER}}` | The per-worker DB/cache wrapper script. |
| `{{BACKEND_SETUP_COMMAND}}` / `{{BACKEND_BOOT_COMMAND}}` / `{{LOCAL_BACKEND_BOOT}}` | Preparing and booting the backend in test mode. |
| `{{BACKEND_WORKTREE_ROOT}}` | Where per-worker backend worktrees live. |
| `{{PORT_BASE}}` / `{{PORT_RANGE}}` | The first port and the size of the range a worker's backend port is hashed into from its slug. |
| `{{SLUG}}` | The unique per-worker key that ports, DB names and sockets derive from. |
| `{{HOST_STATE_DIR}}` / `{{REPORTS_DIR}}` / `{{SKILLS_DIR}}` | Host folders for run state, written reports and installed skills. |
| `{{COORDINATOR_TELL}}` | How a worker messages its coordinator. |
| `{{AUTHOR_IDENTITY}}` | The git author identity agents commit as. |

**Seeding, auth and async**

| Placeholder | Meaning |
|---|---|
| `{{SEED_MECHANISM}}` / `{{SEED_ENDPOINT}}` / `{{SEED_DIR}}` | How tests ask the backend for named scenarios; the endpoint; the folder of seed scripts. |
| `{{ISOLATION_STRATEGY}}` | Truncate-before-each-test with serial workers, or a fresh tenant per test with parallel workers. |
| `{{DRAIN_COMMAND}}` | The one sanctioned way to run queued background work deterministically. |
| `{{ASYNC_JOB_SYSTEM}}` / `{{ASYNC_TEST_FLAG}}` | The job system; the env flag that switches its adapter for e2e. |
| `{{AUTH_MECHANISM}}` | How a test signs in programmatically. |
| `{{TOKEN_STORAGE_KEY}}` / `{{TENANT_STORAGE_KEY}}` / `{{CONSENT_STORAGE_KEY}}` | Browser-storage keys for the auth token, the active tenant and the cookie-consent choice. |
| `{{TENANT_SWITCH_MECHANISM}}` | How the app switches the active tenant. |
| `{{ADMIN_EMAIL}}` / `{{ADMIN_PASSWORD}}` | Credentials of the per-test admin user. |
| `{{REVIEW_APP_FLAG}}` / `{{HAS_SERVER_ADMIN_UI}}` | The backend env flag enabling a passwordless admin sign-in; whether a server-rendered admin UI exists. |

**Third parties and integration**

| Placeholder | Meaning |
|---|---|
| `{{THIRD_PARTIES}}` / `{{VENDOR_API_DOCS_URL}}` | The vendors to mock; each vendor's API documentation. |
| `{{VENDOR_A}}` / `{{VENDOR_B}}` | See the neutral vocabulary above. |
| `{{MOCK_TOOL}}` | The mock server (the reference used Mockoon). |
| `{{VENDOR_A_MOCK_PORT}}` / `{{VENDOR_B_MOCK_PORT}}` | Local mock ports. |
| `{{VENDOR_A_MOCK_URL_ENV}}` / `{{VENDOR_B_MOCK_URL_ENV}}` | Test-side env vars holding each mock's base URL. |
| `{{VENDOR_A_OUTBOUND_URL_ENV}}` | The backend env var that points its vendor-A client at the mock. |
| `{{INTEGRATION_PIPELINE}}` / `{{INTEGRATION_PROJECT}}` | The async integration pipeline; the self-scoped Playwright project that covers it. |
| `{{GATEWAY_BOOT_COMMAND}}` / `{{SUBSCRIBER_DAEMON_COMMAND}}` | Booting the integration gateway and the backend's message subscriber. |
| `{{GATEWAY_RESET_SCRIPT}}` | The gateway repo's own script that truncates its test databases between tests. |
| `{{GATEWAY_CONTAINER_ENV}}` / `{{GATEWAY_COMPOSE_PROJECT_ENV}}` / `{{GATEWAY_REPO_ENV}}` | Test-side env vars naming the gateway container, its compose project, or a native gateway checkout to run the reset in. |
| `{{GATEWAY_DB_URL_ENV}}` / `{{GATEWAY_SECONDARY_DB_URL_ENV}}` | The gateway's database URL env vars, exported when the reset runs natively. |

**CI and reporting**

| Placeholder | Meaning |
|---|---|
| `{{CI_PLATFORM}}` / `{{CI_BACKEND_ARTIFACT}}` / `{{CONTAINER_REGISTRY}}` | The CI system; the pinned backend artifact CI runs; where it is published. |
| `{{SMOKE_SHARDS}}` / `{{FULL_SHARDS}}` | Shard counts for the smoke and full runs. |
| `{{BASE_WORKFLOW}}` | The reusable CI workflow every Playwright job calls. |
| `{{APPROVAL_WORKFLOW}}` / `{{SMOKE_WORKFLOW}}` / `{{FULL_REGRESSION_WORKFLOW}}` / `{{RELEASE_TESTS_WORKFLOW}}` / `{{INTEGRATION_WORKFLOW}}` | The workflows that add the approval label, run smoke, run the full regression on release PRs, run the release suite on a label, and run the integration project. |
| `{{TIMINGS_REFRESH_WORKFLOW}}` / `{{REPORT_PRUNE_WORKFLOW}}` / `{{PROD_SMOKE_WORKFLOW}}` | The workflows that refresh shard timings, prune published reports, and run the post-deploy production smoke. |
| `{{FULL_REGRESSION_LABEL}}` / `{{LEGACY_RUNNER_FULL_LABEL}}` / `{{RELEASE_TESTS_LABEL}}` / `{{INTEGRATION_LABEL}}` | PR labels that request a full Playwright run, a full run of the legacy runner, the release suite, and the integration project. |
| `{{TIMINGS_FILE}}` / `{{SHARD_SPLITTER_SCRIPT}}` / `{{TIMINGS_MERGE_SCRIPT}}` / `{{DIFF_TAGS_SCRIPT}}` | The per-spec timings file; the scripts that bin-pack specs into shards, merge per-shard timings, and map a PR diff to feature tags. |
| `{{PROD_SMOKE_CONFIG}}` | The separate Playwright config for the production smoke. |
| `{{REPORT_HOSTING}}` | Where merged HTML reports are published. |
| `{{LABEL_POLICY}}` | PR labels that gate or skip test runs. |
| `{{PROD_SMOKE_WANTED}}` / `{{PROD_SMOKE_TARGET}}` / `{{CANARY_IDS}}` | Whether a post-deploy production smoke exists; its target; its canary data. |

**Tracker, chat and intent**

| Placeholder | Meaning |
|---|---|
| `{{TRACKER}}` / `{{TRACKER_PROJECT_KEY}}` / `{{TRACKER_TOOLS}}` | The issue tracker; the team or project key; the agent tools that read and write it. |
| `{{CHAT_TOOL}}` / `{{CHAT_TOOLS}}` | The team chat; the agent tools that read it. |
| `{{CHAT_DEV_CHANNEL}}` / `{{CHAT_QA_CHANNEL}}` | Where developers discuss the product; where QA reports bugs. |
| `{{INTENT_SOURCE}}` / `{{INTENT_SKILL}}` | The written product-intent source (a knowledge base, PRDs); the skill that consults it. |
| `{{BUG_FILING_SKILL}}` | The skill that files a bug in the tracker. |

Bundle and skill files also contain `<angle-bracket>` notes. Those are not placeholders: they describe what content belongs at that spot in the new project.
