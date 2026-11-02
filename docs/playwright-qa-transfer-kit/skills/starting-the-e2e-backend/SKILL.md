---
name: starting-the-e2e-backend
description: Use when you need the {{PROJECT_NAME}} backend up in test mode for an e2e run (Playwright or Cypress) - the backend won't boot in the test env, the seed endpoint returns 404 or 500, you need per-worker DB/cache/port isolation, the backend worktree hits a native-gem ABI or schema-load error, the services wrapper targets the wrong slug, SSO/OAuth login never leaves /login locally, or you are tearing an e2e backend down.
---

# Starting the e2e backend

## Overview

The **backend in test mode** is the foundation both e2e runner skills sit on: **running-e2e-tests**
(Playwright) and `running-cypress-e2e` (not included in this kit). Each runner owns its own Vite and
browser bring-up; this skill covers **only the backend**: per-slug isolation, the boot command, the seed
endpoint, readiness and teardown. It does not start Vite or a browser.

Core rules:

- The backend runs from a **backend worktree of `{{BACKEND_CHECKOUT}}`**, not the frontend repo.
- **One isolated backend per consumer.** Every worker (and every manual run) gets its own database,
  cache, port and worktree, all derived from one unique key, the slug. There is no shared or long-lived
  stack: concurrent seeds against a shared backend pollute each other.
- `{{SERVICES_WRAPPER}}` starts/stops the per-slug database + cache (unix sockets, so no TCP ports to
  coordinate) and exports the connection URLs the backend reads.

> **Integration-project specs** additionally need a multi-service stack (gateway, message queue server, object
> storage emulator, vendor-B mock). See "Self-scoped integration project" near the end.

## Per-worker parameters (compute first)

```bash
slug="{{SLUG}}"
[ -z "$slug" ] && { echo "ABORT: set an explicit short slug" >&2; exit 1; }
be_port=$(printf '%s' "$slug" | cksum | awk '{print {{PORT_BASE}} + ($1 % {{PORT_RANGE}})}')
be_wt={{BACKEND_WORKTREE_ROOT}}-$slug
```

- Use your own task slug. For manual or coordinator runs pass an **explicit short throwaway slug**; never
  let the services wrapper fall back to a default slug that some other session also uses.
- The port is a stable hash of the slug into a fixed range, so two slugs get different ports without any
  registry. DB, cache, port, tmux session name, worktree and log are all slug-keyed.

**Keep the slug SHORT (socket-path length limit).** The per-slug Postgres unix socket path must stay
under the kernel's 108-byte `sun_path` limit, or the database silently fails to init (`initdb` /
`pg_ctl` never bind). Check before provisioning:

```bash
pg_sock_path="{{HOST_STATE_DIR}}/services/$slug/pg/sock/.s.PGSQL.5432"
[ ${#pg_sock_path} -lt 108 ] || { echo "ABORT: socket path ${#pg_sock_path}B >= 108 for slug '$slug'; use a shorter slug" >&2; exit 1; }
```

## Preconditions (verify before starting)

1. **Backend worktree** exists at `$be_wt`. Two provisioning modes; both create the slug's isolated
   DB/cache and load the schema (`{{BACKEND_SETUP_COMMAND}}`; do not skip it - a fresh worktree's DB is
   empty until the schema loads). Dependencies (e.g. the gem bundle, node_modules) are shared from the
   main checkout.

   **(a) Default: track `{{BACKEND_CHECKOUT}}`'s current HEAD.** The worktree must sit on the **same
   commit** as the main checkout; if it drifts, the shared native gems ABI-mismatch on schema load.
   ```bash
   git -C {{BACKEND_CHECKOUT}} worktree add $be_wt $(git -C {{BACKEND_CHECKOUT}} rev-parse HEAD)
   cd $be_wt && SPORE_TASK_SLUG=$slug {{BACKEND_SETUP_COMMAND}}
   ```

   **(b) Branch-aware: check out a companion backend branch** when your task ships backend seed/support
   changes tied to a frontend PR (author, commit and push them there). The **ABI guard is mandatory**:
   the shared bundle is built against the main checkout's lockfile, so hard-stop on any lockfile diff
   rather than silently sharing a mismatched bundle.
   ```bash
   be_branch="<companion-branch>"
   main_wt={{BACKEND_CHECKOUT}}
   {{SECRETS_WRAPPER}} git -C "$main_wt" fetch origin "$be_branch" || { echo "fetch failed" >&2; exit 1; }
   if ! diff -q <(git -C "$main_wt" show HEAD:Gemfile.lock) \
                <(git -C "$main_wt" show "origin/$be_branch:Gemfile.lock") >/dev/null 2>&1; then
     echo "ABI GUARD: origin/$be_branch lockfile differs from main HEAD -> STOP and escalate." >&2
     echo "  Opt-in escape hatch: a worktree-LOCAL bundle (bundle config set --local path ...; bundle install)." >&2
     exit 1
   fi
   git -C "$main_wt" worktree add -B "$be_branch" "$be_wt" "origin/$be_branch"
   cd "$be_wt" && SPORE_TASK_SLUG=$slug {{BACKEND_SETUP_COMMAND}}
   ```

   **"already used by worktree" on mode (b):** another slug already has a worktree on that branch (git
   refuses to check one branch out twice). Do not delete or force past it. Inspect it
   (`git -C <other_path> status --short`, `rev-parse HEAD`), compare with the fetched
   `origin/<branch>`, and if clean and matching, **reuse it**: either point your own slug's DB/cache/port
   at that path, or, if the owning task is finished, reuse its slug after clearing its stale service
   state.

   (`SPORE_TASK_SLUG` is the variable the reference services wrapper reads; use whatever yours reads.)

2. Login and most specs do **not** touch external services. **Vendor-A-path specs DO** - any spec that
   asserts the distribution API received a request (publish / republish / stop / pause / archive /
   activate). For those you MUST boot the vendor-A mock ("Vendor-A mock" below). Without it the backend's
   outbound call is connection-refused, the item lands in a failed state, and the spec fails looking like
   a product bug.

## Start the backend (idempotent)

Own tmux session, slug-suffixed. **Check first**; if curl connects, skip bring-up. Only kill a session
carrying YOUR slug.

```bash
curl -s -o /dev/null --max-time 3 http://localhost:$be_port/ && echo up

tmux kill-session -t e2e-backend-$slug 2>/dev/null || true
tmux new-session -d -s e2e-backend-$slug -x 220 -y 50
tmux send-keys -t e2e-backend-$slug "cd $be_wt && SPORE_TASK_SLUG=$slug {{SERVICES_WRAPPER}} exec env RAILS_ENV=test {{ASYNC_TEST_FLAG}}=true WEB_CONCURRENCY=0 <queue-redis-url-env>=<per-slug socket url> {{VENDOR_A_OUTBOUND_URL_ENV}}=http://localhost:{{VENDOR_A_MOCK_PORT}} {{BACKEND_FRONTEND_URL_ENV}}={{FRONTEND_URL}} APP_URL=http://localhost:$be_port {{BACKEND_BOOT_COMMAND}} -p $be_port -e test 2>&1 | tee /tmp/e2e-backend-$slug.log" Enter
```

- **The test env is load-bearing.** The seed endpoint mounts automatically in the test env (reference:
  cypress-on-rails in the test gem group). Booting in any non-test env breaks seeding.
- **`APP_URL` (or whatever builds absolute callback URLs) MUST match `$be_port`, on every boot.** A
  committed test env file that pins it to CI's fixed port makes the OAuth callback redirect to a dead
  port on every hashed-port backend: the browser never leaves `/login`, nothing errors, and it looks like
  broken SSO or a flaky assertion. CI is immune (it uses the fixed port), so it reproduces only locally.
- **Pass every connection URL the wrapper does NOT export.** A wrapper may export the main cache URLs
  but not, say, the job-queue Redis URL; the committed test env file's `localhost` default then wins and
  every seed 500s with a connection error. Audit the committed test env file for localhost defaults and
  pass the per-slug value inline.
- **Single-thread variant** (needed by specs that replay recorded HTTP via VCR, whose cassette state is
  thread-local; under multi-thread Puma the serving thread often has no cassette -> `VCR::Errors::UnhandledHTTPRequestError: There is currently no cassette in use`):
  add `RAILS_MIN_THREADS=1 RAILS_MAX_THREADS=1 AR_POOL_SIZE=5`. Playwright does not need it; the
  Cypress runner sets it always.

**Wait for readiness** without sleeping in your own shell: push the loop into the pane and watch for a
sentinel.

```bash
tmux send-keys -t e2e-backend-$slug "until curl -s -o /dev/null --max-time 3 http://localhost:$be_port/; do sleep 2; done; echo BACKEND_READY" Enter
tmux capture-pane -p -t e2e-backend-$slug | grep -q BACKEND_READY && echo up
```

Readiness = curl *connects* (200/302/404 all mean up). If the sentinel never appears, read
`/tmp/e2e-backend-$slug.log`.

## Vendor-A mock ({{MOCK_TOOL}} on `{{VENDOR_A_MOCK_PORT}}`) - vendor-A-path specs only

CI boots a `{{MOCK_TOOL}}` server with the vendor-A mock data file from `{{MOCKS_HOME}}` and exports
`{{VENDOR_A_MOCK_URL_ENV}}`; locally you must do the same or the backend's outbound call is refused. The
backend is already pointed at it (`{{VENDOR_A_OUTBOUND_URL_ENV}}` in the boot line).

- **Mock data:** a checkout of `{{MOCKS_HOME}}`; refresh it with `git pull` before use.
- **CLI version:** pin the mock CLI to the exact version CI uses; bump in lockstep with the workflow.
- **Port isolation:** a fixed mock port is safe only inside a per-slug network namespace; for parallel
  workers without one it collides. Boot the mock in the SAME namespace/pane context as backend + runner so
  `localhost:{{VENDOR_A_MOCK_PORT}}` resolves for both.

```bash
tmux kill-session -t e2e-mock-$slug 2>/dev/null || true
tmux new-session -d -s e2e-mock-$slug -x 200 -y 50
tmux send-keys -t e2e-mock-$slug "cd {{MOCKS_HOME}} && exec {{DEVSHELL}} npx --yes <mock-cli>@<ci-version> start --data <vendor-A mock file> --port {{VENDOR_A_MOCK_PORT}}" Enter
tmux send-keys -t e2e-mock-$slug "until curl -s -o /dev/null --max-time 3 http://localhost:{{VENDOR_A_MOCK_PORT}}/health; do sleep 1; done; echo MOCK_READY" Enter
```

Then verify one real route returns 200 and the mock's admin transaction-log API answers.

- **Runner env:** the Playwright runner must export `{{VENDOR_A_MOCK_URL_ENV}}`; the mock helper
  (`{{E2E_ROOT}}support/vendorAMock.ts`, via the fixtures) reads/purges transaction logs there. If unset,
  purges silently no-op and log reads throw. A global setup purges logs at suite start.
- **Teardown:** `tmux kill-session -t e2e-mock-$slug`.

## Seed endpoint (`{{SEED_ENDPOINT}}`)

Both runners seed/reset the DB by POSTing to `{{SEED_ENDPOINT}}`. **Read the response body - it
separates three very different failures:**

- **Command not found** - a 404 with a JSON body like `{"message":"could not find command file: .../<name>.rb"}`:
  the endpoint is LIVE; the command name is wrong or its file is missing. Look in `$be_wt/{{SEED_DIR}}`
  for the right name.
- **Not mounted** - a 404 that is a router/HTML page: the backend is not in the test env, not up, or
  `BACKEND_URL` points elsewhere. Boot with `RAILS_ENV=test ... -e test`. Sanity check:
  `bundle exec rails runner 'p Rails.application.middleware.map(&:name).grep(/Cypress/i)'` lists the
  seed middleware.
- **Script raised** - a 500: the command file was found and executed but the seed code raised. Read the
  named file under `$be_wt/{{SEED_DIR}}` and the backend log; use **investigating-e2e-failures**.

A seed failure in `beforeEach` is NOT an assertion failure; localize the layer before calling a red run a
test bug.

## Teardown / leave as found

Tear down only what **you** started (if it was up when you arrived, leave it). Stop without wiping data:
`down`, NOT `reset`.

```bash
cd $be_wt && SPORE_TASK_SLUG=$slug {{SERVICES_WRAPPER}} down
tmux kill-session -t e2e-backend-$slug
```

- **Every slug:** run `down` at task end and remove your per-slug worktree
  (`git -C {{BACKEND_CHECKOUT}} worktree remove $be_wt`). Orphaned per-slug stacks (app server, Redis,
  Postgres) accumulate and exhaust host memory; check the process tree for leftovers you own.
- **Mode (b):** **push first** - removing the worktree discards unpushed commits on its local branch.
- **A server that traps SIGTERM needs a hard kill.** If your app server (reference: Puma) ignores a
  plain `kill`, confirm the PID is YOURS (its cwd / env carries your slug), then `kill -9` it. Never
  pattern-kill by a process name others share.

## Self-scoped integration project (outline)

For specs that drive an `{{INTEGRATION_PIPELINE}}` end to end: an inbound event hits the backend, the
backend fans out to a separate **gateway** service over a **message queue server**, the gateway calls
`{{VENDOR_B}}` (a `{{MOCK_TOOL}}` fake on `{{VENDOR_B_MOCK_PORT}}`), optionally moves bytes through an
**object-storage emulator** (reference: LocalStack S3, or MinIO as a native substitute), and a
**subscriber daemon** writes results back into the backend. The spec asserts the outcome through the real
UI. This skill owns the stack; the invocation lives in **running-e2e-tests** -> "Integration project run".

Recipe outline (fill each placeholder for your stack):

1. **One network namespace for everything TCP.** Gateway, queue server, storage emulator, backend, mocks, Vite
   and Playwright must all reach each other. If the gateway runs in containers with host networking, they
   join the host namespace; the agent's own shell may be in a different, isolated namespace where none of
   them answer ("listening" but curl returns 000). Boot everything in tmux panes in the shared namespace
   and run readiness checks + Playwright from there. Unix-socket DB/cache are namespace-independent.
2. **Gateway stack:** `{{SECRETS_WRAPPER}} {{GATEWAY_BOOT_COMMAND}}` (fresh gateway DB each session: tear
   down with volumes, then up). Readiness = the queue the gateway consumes reports a consumer. On a
   fresh host, build the image early: a branch only ever built in CI can miss system build libraries for
   native extensions locally.
3. **Reset gateway state between tests and runs.** If the gateway DB is not truncated between seeds and a
   primary-key sequence starts behind fixture rows, the Nth create RPC collides on the key, never replies,
   and the backend's RPC client times out -> the seed POST returns 500. Truncate (restart identity) all
   non-fixture tables per test and keep fixture-table sequences aligned.
4. **Mocks:** boot `{{VENDOR_B}}`'s mock (always) and vendor A's (only if the flow reaches it). **Boot
   with `exec` and no `| tee`**: a piped mock session can die after one request; the `exec` form stays up.
5. **Backend:** branch-aware worktree (mode b), its own slug, `{{ASYNC_TEST_FLAG}}=true` (so async
   callbacks actually run), the queue-server URL, `{{VENDOR_B_MOCK_URL_ENV}}`, and for storage flows the storage
   endpoint env plus a short attach-poll interval so async write-back converges inside the test timeout.
6. **Subscriber daemon:** `{{SUBSCRIBER_DAEMON_COMMAND}}` in its own pane, **same env as the backend**.
   Boot it for EVERY integration spec: any spec asserting a gateway-to-backend write-back fails without it,
   looking like a product bug (the UI keeps the factory value). Run **exactly one**; two consumers on the
   queue means a duplicate, and killing the wrong one can leave zero correctly configured consumers.

Integration rules:

- **Pass env INLINE**, never via a generated env file the services wrapper regenerates on every `up`, and
  check that the committed test env file does not silently resolve a missing value to a real remote
  resource. Confirm env actually reached a process with `tr '\0' '\n' < /proc/<pid>/environ | grep <VAR>`
  before trusting a green or debugging a red.
- **Use `127.0.0.1`, not `localhost`, on the Node/Playwright side** (IPv4-only mocks vs `::1`). Server-side
  env that already works may keep `localhost`.
- **The stack must be up before the spec seeds** when the seed fires a synchronous RPC; there is no
  reachability pre-check, so a down stack fails the spec at its first interaction.
- A missing storage env shows up as a bare test timeout with no diagnostic. Prove a storage round-trip by
  enumerating the bucket after the run, not by the green tick alone.

**Teardown:** gateway down with volumes, kill the mock/backend/daemon panes you started, `down` for the
integration slug. Don't remove a worktree another run may still be using.

## Common mistakes

| Mistake | Fix |
|---------|-----|
| Letting the services wrapper default its slug | Always pass your slug explicitly for up/down/status/exec. |
| Expecting a shared backend on a fixed port | Every run provisions its OWN worktree, DB, cache and hashed port. |
| Schema load fails with an ABI / native-extension error | `$be_wt` drifted off the main checkout's commit (or lockfile). Realign (Preconditions). |
| Long slug, Postgres never comes up | Socket path over 108 bytes; use a shorter slug. |
| SSO login stuck on `/login` locally only | Pass `APP_URL=http://localhost:$be_port`. |
| Every seed 500s with a Redis connection error | A connection URL the wrapper doesn't export fell back to a localhost default; pass it inline. |
| VCR "no cassette in use" | Boot single-thread. |
| Plain `kill` leaves the server running | It traps SIGTERM; verify ownership, then `kill -9`. |
