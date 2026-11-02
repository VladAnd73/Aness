# playwright/tests/integration - Integration-as-user e2e harness (`--project=integration`)

Drives the **real** {{INTEGRATION_PIPELINE}} the way a tenant's upstream system does - the backend
receives a {{VENDOR_B}} order webhook, fans out to the **real integration gateway** over a message queue
(RabbitMQ), and the gateway calls {{VENDOR_B}} (a {{MOCK_TOOL}} fake). The harness asserts that the
outbound vendor calls succeeded (their response status) **and** that the placed order shows up activated
on the real Order Management page a member sees - through the app's own UI and GraphQL, not the backend
console or a banner. Parent: [`../CLAUDE.md`](../CLAUDE.md).

Selected via `--project=integration`. The integration project is always registered in
`playwright.config.ts`, but its own `testDir` self-scopes it and the `chromium` project `testIgnore`s this
folder, so the always-on main suite never collects these specs nor tries to seed against a stack that
isn't booted. `{{INTEGRATION_WORKFLOW}}` runs `--project=integration`.

> **Cross-repo dependencies and first-run status.** <State which backend seeds and gateway boot script
> this harness needs and whether each is on its base branch.> The workflow pins the gateway checkout by
> commit deliberately - it hands secrets to code it then executes - and is gated on the
> `{{INTEGRATION_LABEL}}` label. **Until the workflow has a recorded green run, treat a first red run as a
> possible harness defect, not a regression.** When you add an integration spec, check what it needs
> against that workflow, and state any dependency that is not yet on a base branch in the PR.

## What's here
| File | Exercises | @smoke |
|------|-----------|--------|
| <spec file> | <`Integration \| <flow>` - the scenario in one line, incl. the terminal state the member lands on> | <yes/no> |

<One row per spec: the base order, each order variant (e.g. a different lifecycle state or record kind),
the import path, and any rule spec with a `test.fixme` for a known open bug.>

## Live write-back & per-variant upstream ids

<Describe the write-back daemon (one daemon consumes the gateway's notifications for the whole stack) and
how each spec depends on it.> For an **order**, the gateway fetches the upstream record from the vendor
mock and the subscriber re-derives the ordered record's fields from it - asynchronously, so it does
**not** block the order's own fetches, but it *does* overwrite the seeded record's fields by the time a
spec reads the record page.

So each order variant fetches its **own** upstream id and the mock returns a matching upstream record:

| Variant (`seedVendorOrder`) | upstream id | upstream shape -> write-back lands |
|---|---|---|
| <base `{}`> | <REC-ORDER-BASE> | <the fields that drive the re-derivation -> the resulting state/kind> |
| <one row per variant option> | <REC-ORDER-...> | <...> |

A single shared upstream id cannot serve two variants whose upstream shapes differ, hence per-variant ids
(mirroring the import path's per-kind ids). <Note any variant whose mapping is non-obvious, e.g. a kind
that maps to a state the order path never fetches, and what the spec asserts instead.>

## The stack (backend + Vite native, the integration-gateway stack in Docker)

```
Playwright integration project --HTTP--> backend (test env) --RabbitMQ RPC--> integration gateway (Docker) --> {{VENDOR_B}} mock ({{VENDOR_B_MOCK_PORT}})
   | asserts                              |  inbound order webhook (Basic)         | HTTP client          `--> LocalStack S3 (:4566)
   | mock txn-log                         |  {{SEED_ENDPOINT}} (seed) + /api/graphql
   v                         write-back daemon <--RabbitMQ notify-- integration gateway
                             (backend daemon, test env - re-derives the ordered/imported record)
```

On {{HOST_OS}} the **integration-gateway stack runs in Docker** (the gateway repo's e2e Docker boot
script, host-network compose): the gateway itself plus RabbitMQ (`:5672`), Postgres, Redis and LocalStack
S3 (`:4566`). The backend, Vite and the vendor B mock run **natively**; so does the backend write-back
daemon, which consumes the gateway's notifications and re-derives the ordered/imported record (see the
write-back section above). <Say whether an all-native path exists on your host.>

| Service | Where | Port | Notes |
|---|---|---|---|
| backend | native, {{BACKEND_REPO}} `{{DEFAULT_INTEGRATION_BRANCH}}` | {{BACKEND_PORT}} (any free) | test env, per-slug PG/Redis via `{{SERVICES_WRAPPER}}` |
| integration-gateway stack | **Docker**, gateway repo (its e2e Docker boot script) | RabbitMQ 5672, LocalStack S3 4566 | one host-network compose: gateway + RabbitMQ + Postgres + Redis + LocalStack; blocks until the RPC queue has a consumer |
| backend write-back daemon | native | - | test env + `{{ASYNC_TEST_FLAG}}`; consumes gateway notifications and runs the receive job (order-path re-derivation + import writes) |
| {{VENDOR_B}} mock | native, {{MOCKS_HOME}} | {{VENDOR_B_MOCK_PORT}} | No auth rule (the mock ignores the inbound Basic creds); admin txn-log at the mock tool's admin path |
| frontend (Vite) | native, this repo | {{FRONTEND_PORT}} | serves the Order Management page the UI assertion drives (`{{BACKEND_URL_ENV}}` -> backend) |

## Boot

The canonical, reproducible boot recipe is the repo README section on running the integration-as-user
e2e tests - keep this file and that section in sync; don't fork a second recipe here. The
integration-suite-specific facts that matter when you touch these specs:

- **Bring up the Docker gateway stack first.** The seed fires the create-tenant RPC **synchronously**, so
  the gateway (and its RabbitMQ) must be up **before** the integration specs seed, or the seed rolls back
  and the spec fails loudly at its first interaction (there is no reachability pre-check).
- **`{{ASYNC_TEST_FLAG}}` is required** on both the backend server and the write-back daemon: it switches
  the backend's job adapter from the inline test adapter to an async one, so the order-status callback
  (the job that POSTs the order status to the vendor mock) and the write-back job actually run - otherwise
  they only enqueue and the assertions time out.
- **The test environment is required** on the write-back daemon (its binstub may default to
  development): otherwise the daemon runs against the dev DB, never loads the test env file, and silently
  processes nothing - the import spec times out and the order specs read the un-re-derived seed.
- **Same netns:** the natively-booted TCP services (backend, Vite, mock) and the Playwright run must share
  one network namespace - boot them in the **same tmux server** and run Playwright there too; the Docker
  gateway stack is reached over the host network (`localhost`). PG/Redis use unix sockets, so they are
  netns-independent.
- **Per-test gateway DB reset follows the gateway's topology.** The `resetIntegrationGateway` fixture
  runs the gateway repo's own `{{GATEWAY_RESET_SCRIPT}}` - `docker exec`ing the container by default, or
  directly in a checkout when `{{GATEWAY_REPO_ENV}}` is set. Set it (with the test env + the
  `{{GATEWAY_DB_URL_ENV}}` / `{{GATEWAY_SECONDARY_DB_URL_ENV}}` its `{{SERVICES_WRAPPER}}` prints) whenever the gateway
  runs **natively** rather than in Docker - a run whose network namespace cannot reach host-network
  containers has no container to exec into. Either way it is the same script; never add a second reset
  implementation.
- **Vite on {{HOST_OS}}:** <any host-specific Vite boot workaround (e.g. a dev-server plugin that must be
  disabled in a nested worktree)>. Only needed when the `--project=integration` run includes a
  UI-asserting spec.

## Run

```bash
BACKEND_URL={{BACKEND_URL}} \
{{VENDOR_B_MOCK_URL_ENV}}=<vendor B mock URL> \
FRONTEND_URL={{FRONTEND_URL}} \
  yarn playwright test --project=integration
```

No webhook Basic-auth env is needed when the fixture and the backend's test env default to the same
dev-only credential pair, so the enforced inbound-webhook Basic auth lines up on its own. On {{HOST_OS}},
wrap the command in {{DEVSHELL}}.

The single `integration` project **imports the shared `test` fixture** (`~/playwright/support/fixtures`,
the same one every main-area spec uses) - the real `Record` / `OrderManagement` page objects and
`userFlows` - which also carries the integration-specific fixtures: `integrationOrders` /
`integrationImports` (the integration backend drivers) and `vendorBMock` (vendor B outbound-contract
assertions), lazily instantiated so only integration specs pay for them. The integration pieces are
extended into that one shared fixture in place - **do NOT re-create a `tests/integration/support/`
fork**; new integration fixtures/helpers go in [`../../support/`](../../support/CLAUDE.md). An
integration helper must stay **dep-clean** (import only `@playwright/test`, never app-source barrels) or
the integration project fails to load - that's why the sign-in / seed / GraphQL primitives live in the
dep-clean `support/integrationBackend.ts` + `support/backendApi.ts` (the shared low-level request layer
that `Server`, `UserFlows`, and `IntegrationBackend` all delegate to), not in the barrel-tainted `Server`.
The vendor B mock's state + transaction log are purged **before** every test (no teardown purge) by the
shared base auto-fixture (`playwright/support/fixtures.ts`), gated on `{{VENDOR_B_MOCK_URL_ENV}}` so it
no-ops on every non-integration run, so each test's outbound-call assertions see only its own traffic.
It drives the backend and the vendor B mock over absolute-URL API contexts and asserts the placed order
through the real UI (Vite). No backend-console `eval`: the order's instance uuid + preview token come
from the app's own GraphQL (the bundle-instances query + the preview-token mutation).

## Seed (committed, not ad-hoc)

`integrationOrders.seedVendorOrder()` (`~/playwright/support/integrationOrders.ts`) runs the committed
backend seed via the standard `{{SEED_ENDPOINT}}` path: `clean` (truncate - the order path resolves its
configuration by a tenant integration id across all order-capable tenants, so a stale duplicate would make
bundles ambiguous) then `seeds/integration/vendor_b_order` (on the backend companion branch). It returns
the seed's **own** JSON (bundle uuid / tenant integration id / upstream id / member login) typed as
`VendorOrderSeed`. That seed creates the vendor B integration configuration via the backend's own create
service, which fires the create-tenant RPC that propagates the vendor's base URL to the gateway (backend =
single source of truth).

The order-capable tenant, its vendor configuration, the orderable bundle and the member login are built by
one shared seed builder, `load`ed by every order-shaped seed - extend that builder rather than branching
either seed further.

**A seed must never trigger the sync it is a precondition for - a user action must.** These specs exist
to prove what a member sees after *they* act, so no integration seed calls the sync job, a fetch RPC, or
any other functionality entry point. <Name the user surface that starts the sync here (e.g. an order whose
first step is the sync, so the first order both kicks the sync and snapshots pre-sync data), and which
entity kinds it can sync.>

**A write-back that the gateway represents in more than one leg is order-dependent - do not assert a
write-back-derived field once.** When the gateway fetches part of a record as a *separate* command event,
it represents the record once without that part and again once it lands, and whichever lands last wins.
A one-shot assertion can therefore go green with the wrong value sitting in the database. For a field
snapshotted at a point in time, wait for the late leg before acting: `integrationOrders.awaitWriteBack(id)`
-> `seeds/integration/await_write_back`. Key the barrier on a marker only the late leg sets **and that is
not the value under test**, so it stays valid under both the bug and its fix. On timeout it reports what it
observed rather than a bare "never landed".

Re-syncing is NOT a reliable substitute: the gateway may skip representing a record it considers fresh.
Any new integration spec that reads a write-back-derived field has the same exposure.

**Rule specs isolate one variable per test.** When a rule spec has several variants (via
`seedVendorExistingRecordLink(variant)`), **the upstream record is the only difference between the
tests**:

| `variant` | upstream id | shape | the rule it isolates |
|---|---|---|---|
| <variant> | <REC-...> | <the upstream fields, incl. any deliberate decoy (e.g. a newer inactive entry next to the active one, so picking by recency instead of activity state fails)> | <the one rule this test proves> |

Make sure the mock fixture can actually reach the branch each variant targets - on a default mock response
a fallback branch may be unreachable and the spec would pass by construction. Variants that compare two
kinds share ONE upstream response so the kind is the only variable between them.

**Pin the backend tip the rule needs.** <Name the minimum backend change these specs require; before it,
one variant fails for the wrong reason and another passes without exercising the gate. Do not run these
specs against an earlier tip.>

Two things about such a seed are load-bearing:

- **Seed the record in the pre-sync state and with a STALE derived value.** When the identity change and
  the derived value arrive from the same write-back, the page changing is what tells the member the sync
  landed, and neither the expected value nor the identity fields can be satisfied by a value the seed
  supplied.
- **Configure the bundle factory onto the branch under test via its parameter.** A factory default that
  pins a field to a literal can route around the only branch that reads the synced record - on that
  default the spec would pass by construction. Use the parameter; do not post-patch what a factory just
  built.

When the order is what syncs, the tests order **twice** (a shared `placeOrder` helper): the first order
kicks the sync and snapshots pre-sync data, and the second - after the write-back lands - is the one whose
item carries the value under test. That is production's own answer to the race (the member who ordered
too early orders again), not a test-only device.

**The expected value is derived from the RULE, never read back from the seed or the mock.** Build the
expected URL from the upstream identifier, or render the member's template, so a wrong value the system
computed for itself still fails; additionally assert the rendered value carries no preview / secret
marker. Where WHICH upstream entry the gateway picked has no UI surface, assert the marker off the
write-back barrier - choosing a fixture whose candidate entries carry distinct values so the assertion
discriminates all of them.

### Known-bug rule spec - scope

<For any `test.fixme` rule spec, record:>
- **Do not "fix" the spec by tightening the wrong layer.** <Where the root cause lives, and why gating a
  downstream adapter harder would regress intended behaviour.>
- **Root cause:** <the component and the decision that is wrong, in one paragraph.>
- **Why the sink is where it is:** <which adapter admits the value and why that is intended for some kinds.>
- **Out of scope on purpose:** <shapes removed because real upstream traffic never delivers them; what new
  evidence would be needed to re-add them.>
- **Not covered on purpose:** <analogous paths in other vendors tracked separately; paths that cannot
  carry the value at all.>

## What the smoke asserts (real outcomes, not a banner)

The base order smoke seeds the order tenant, signs the member in (`userFlows`), fires one order (the
inbound webhook, Basic auth) with a fresh random `order_id`, and asserts:

1. the webhook accepts the request (`200`);
2. **outbound proof the real gateway ran** - the vendor B mock received the record fetch `-> 200`, the
   record's sub-resource fetch `-> 200`, and the backend's order-status callback `-> 204`, via
   `vendorBMock.expect*` helpers asserting the **response status** (a 404 fall-through would fail),
   matched by this run's own ids after a per-test mock state+log purge;
3. **terminal UI state where the member sees it** - `integrationOrders.resolveActivatedOrder` finds the
   created instance via the authenticated bundle-instances query and mints the page token via the same
   preview-token mutation the main-area "Order Management" menu calls; the spec opens the login-less
   Order Management page (`/orders?uuid=...&token=...`) and asserts the **activated** section via the
   real `OrderManagement` PO (bundle name, scheduled status, item count);
4. **member cross-navigation** - following the "manage in app" link lands the signed-in, tenant-scoped
   member on the main-area record page (`recordPage`).

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | shared fixture + driver/helper classes [`../../support/CLAUDE.md`](../../support/CLAUDE.md) | main-area specs [`../main/CLAUDE.md`](../main/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
