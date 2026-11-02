# CLAUDE.md bundle manifest

Every file in this bundle, with the tier it was given after a full read of its source and the placeholders it contains. Paths mirror the reference suite's tree, with its domain folders renamed to the kit's neutral vocabulary (see the kit README).

Tiers:
- **generic**: conventions that transfer with only path renames. Resolve placeholders and install.
- **mixed**: transferable conventions interleaved with the reference's specifics, which are now placeholders or neutral names. Keep the conventions; replace the domain lines with the new project's.
- **project-specific**: a structural outline only (title shape, headings, `<angle-bracket>` notes on what content goes there, any generic rule found in the source, and the standard footer). Use it as a shape example.

Install each file under the new project's e2e root as `CLAUDE.md` (and each `*.tmpl.mdc` as `*.mdc` in the editor's rules folder). Links inside the bundle already use the installed names.

- generic: 8 files
- mixed: 27 files
- project-specific: 60 files

| File | Tier | Placeholders |
|---|---|---|
| `CLAUDE.tmpl.md` | mixed | BACKEND_PORT, BACKEND_REPO, BACKEND_URL, BACKEND_URL_ENV, E2E_ROOT, FRONTEND_ENV_VARS, FRONTEND_PORT, PROJECT_NAME, TOKEN_STORAGE_KEY |
| `editor-rules/bestpractices.tmpl.mdc` | mixed | FORMS_LIBRARY, I18N_LIBRARY, TRACKER |
| `editor-rules/documentation.tmpl.mdc` | generic | E2E_ROOT |
| `editor-rules/testing.tmpl.mdc` | mixed | E2E_ROOT |
| `playwright/CLAUDE.tmpl.md` | mixed | ASYNC_TEST_FLAG, BACKEND_BOOT_COMMAND, BACKEND_REPO, BACKEND_URL, BACKEND_URL_ENV, DEFAULT_INTEGRATION_BRANCH, DEVSHELL, DRAIN_COMMAND, FRONTEND_PORT, FRONTEND_URL, FULL_REGRESSION_LABEL, FULL_SHARDS, HOST_OS, INTEGRATION_PIPELINE, INTEGRATION_WORKFLOW, LEGACY_RUNNER_FULL_LABEL, PROD_SMOKE_CONFIG, PROD_SMOKE_WORKFLOW, PROJECT_NAME, RELEASE_BRANCH_PATTERN, SEED_DIR, SEED_ENDPOINT, SHARD_SPLITTER_SCRIPT, SMOKE_SHARDS, TIMINGS_FILE, TIMINGS_REFRESH_WORKFLOW, VENDOR_A, VENDOR_B |
| `playwright/data/CLAUDE.tmpl.md` | generic | BACKEND_REPO, SEED_ENDPOINT |
| `playwright/data/factory/CLAUDE.tmpl.md` | generic | none |
| `playwright/data/factory/frozenDomain/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/factory/frozenDomain/group/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/factory/frozenDomain/group/subunit/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/factory/member/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/factory/user/CLAUDE.tmpl.md` | mixed | none |
| `playwright/data/fixtures/CLAUDE.tmpl.md` | generic | none |
| `playwright/data/fixtures/common/CLAUDE.tmpl.md` | generic | none |
| `playwright/data/fixtures/files/CLAUDE.tmpl.md` | generic | none |
| `playwright/data/fixtures/Responses/CLAUDE.tmpl.md` | mixed | none |
| `playwright/data/models/CLAUDE.tmpl.md` | generic | none |
| `playwright/data/models/general/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/models/member/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/data/models/user/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/components/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/components/drawers/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/contact/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/dashboard/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/filters/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/group/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/group/structures/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/group/subunits/CLAUDE.tmpl.md` | project-specific | VENDOR_B |
| `playwright/pageObjects/components/drawers/item/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/media/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/member/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/posts/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/record/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/drawers/user/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/components/headers/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/modals/actionsMenu/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/components/modals/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/modals/confirmation/CLAUDE.tmpl.md` | generic | none |
| `playwright/pageObjects/components/modals/entity/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/modals/orders/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/sections/blocks/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/sections/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/sections/drawer/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/sections/group/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/components/sections/list/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/pages/Account/CLAUDE.tmpl.md` | mixed | none |
| `playwright/pageObjects/pages/AdminPanel/CLAUDE.tmpl.md` | mixed | BACKEND_PORT, FRONTEND_PORT, REVIEW_APP_FLAG |
| `playwright/pageObjects/pages/CLAUDE.tmpl.md` | mixed | PROJECT_NAME |
| `playwright/pageObjects/pages/Contact/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Dashboard/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Group/Analytics/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Group/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Group/items/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Group/subunits/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Item/CLAUDE.tmpl.md` | mixed | DRAIN_COMMAND, VENDOR_A |
| `playwright/pageObjects/pages/Login/CLAUDE.tmpl.md` | mixed | TOKEN_STORAGE_KEY |
| `playwright/pageObjects/pages/Member/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Orders/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/Records/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/pageObjects/pages/ReportApp/CLAUDE.tmpl.md` | mixed | CANARY_IDS, PROD_SMOKE_TARGET, PROJECT_NAME |
| `playwright/prodSmoke/CLAUDE.tmpl.md` | mixed | CANARY_IDS, DEFAULT_INTEGRATION_BRANCH, PROD_SMOKE_CONFIG, PROD_SMOKE_TARGET, PROD_SMOKE_WORKFLOW, SHARD_SPLITTER_SCRIPT |
| `playwright/support/CLAUDE.tmpl.md` | mixed | BACKEND_PORT, CONSENT_STORAGE_KEY, DRAIN_COMMAND, GATEWAY_COMPOSE_PROJECT_ENV, GATEWAY_CONTAINER_ENV, GATEWAY_DB_URL_ENV, GATEWAY_REPO_ENV, GATEWAY_RESET_SCRIPT, GATEWAY_SECONDARY_DB_URL_ENV, MOCK_TOOL, REVIEW_APP_FLAG, SEED_ENDPOINT, TENANT_STORAGE_KEY, TOKEN_STORAGE_KEY, VENDOR_A, VENDOR_A_MOCK_URL_ENV, VENDOR_B, VENDOR_B_MOCK_PORT, VENDOR_B_MOCK_URL_ENV |
| `playwright/tests/account/CLAUDE.tmpl.md` | mixed | none |
| `playwright/tests/account/users/CLAUDE.tmpl.md` | mixed | none |
| `playwright/tests/auth/CLAUDE.tmpl.md` | mixed | PROJECT_NAME, TOKEN_STORAGE_KEY |
| `playwright/tests/CLAUDE.tmpl.md` | mixed | INTEGRATION_PIPELINE, VENDOR_B |
| `playwright/tests/integration/CLAUDE.tmpl.md` | mixed | ASYNC_TEST_FLAG, BACKEND_PORT, BACKEND_REPO, BACKEND_URL, BACKEND_URL_ENV, DEFAULT_INTEGRATION_BRANCH, DEVSHELL, FRONTEND_PORT, FRONTEND_URL, GATEWAY_DB_URL_ENV, GATEWAY_REPO_ENV, GATEWAY_RESET_SCRIPT, GATEWAY_SECONDARY_DB_URL_ENV, HOST_OS, INTEGRATION_LABEL, INTEGRATION_PIPELINE, INTEGRATION_WORKFLOW, MOCKS_HOME, MOCK_TOOL, SEED_ENDPOINT, SERVICES_WRAPPER, VENDOR_B, VENDOR_B_MOCK_PORT, VENDOR_B_MOCK_URL_ENV |
| `playwright/tests/main/adminPanel/autoOrderAttempts/CLAUDE.tmpl.md` | project-specific | REVIEW_APP_FLAG |
| `playwright/tests/main/adminPanel/CLAUDE.tmpl.md` | mixed | BACKEND_PORT, REVIEW_APP_FLAG, VENDOR_A |
| `playwright/tests/main/adminPanel/failedItems/CLAUDE.tmpl.md` | project-specific | MOCK_TOOL, REVIEW_APP_FLAG, VENDOR_A, VENDOR_A_MOCK_PORT, VENDOR_A_MOCK_URL_ENV |
| `playwright/tests/main/adminPanel/ordersOverview/CLAUDE.tmpl.md` | project-specific | REVIEW_APP_FLAG |
| `playwright/tests/main/adminPanel/vendorContracts/CLAUDE.tmpl.md` | project-specific | MOCK_TOOL, REVIEW_APP_FLAG, VENDOR_A, VENDOR_A_MOCK_PORT |
| `playwright/tests/main/CLAUDE.tmpl.md` | mixed | DRAIN_COMMAND, VENDOR_A, VENDOR_B |
| `playwright/tests/main/contacts/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/dashboard/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/frozenDomain/CLAUDE.tmpl.md` | project-specific | TRACKER, VENDOR_B |
| `playwright/tests/main/frozenDomain/group/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/groups/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/group/subunits/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/group/subunits/list/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/items/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/items/list/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/sidebar/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/topNavbar/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/frozenDomain/topNavbar/createNew/CLAUDE.tmpl.md` | project-specific | TRACKER |
| `playwright/tests/main/item/CLAUDE.tmpl.md` | project-specific | DRAIN_COMMAND, MOCKS_HOME, MOCK_TOOL, VENDOR_A, VENDOR_A_MOCK_PORT, VENDOR_A_MOCK_URL_ENV |
| `playwright/tests/main/item/details/restrictedField/CLAUDE.tmpl.md` | project-specific | ROLES |
| `playwright/tests/main/item/details/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/item/details/edit/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/login/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/member/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/orderManagement/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/record/CLAUDE.tmpl.md` | project-specific | none |
| `playwright/tests/main/sidebar/CLAUDE.tmpl.md` | project-specific | none |
