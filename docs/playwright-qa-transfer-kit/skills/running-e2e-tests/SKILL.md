---
name: running-e2e-tests
description: Use when running or executing the {{PROJECT_NAME}} Playwright end-to-end (e2e) tests - any spec under {{E2E_ROOT}}tests - or when an e2e run fails to launch the browser, cannot reach the backend or the Vite dev server, or a spec is red locally but green in CI.
---

# Running the Playwright e2e suite

## Overview

A Playwright e2e run needs three things up: the **backend in test mode**, the **Vite dev server on
`{{FRONTEND_PORT}}`**, and a **browser**.

- **Backend:** start it via the **starting-the-e2e-backend** skill (the shared backend layer, also used
  by the Cypress runner). Backends are per-slug and parallel; that skill owns isolation, ports,
  worktree, seeding and teardown. Playwright needs **no single-thread flag**.
- **Vite + browser (this skill):** two non-obvious rules make this layer reliable:

1. **Run the `playwright test` command inside the pinned dev shell (`{{DEVSHELL}}`).** It provides a
   browser whose revision matches the repo's `@playwright/test` (via `PLAYWRIGHT_BROWSERS_PATH`). With
   it you need **zero** workaround env: no patchelf, no `PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS`, no
   `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD`.
2. **The Vite + Playwright layer is not parallel by default** (see Concurrency limit): only ONE Vite +
   Playwright run per frontend port, even though backends run side by side.

Frontend checkout: `{{FRONTEND_CHECKOUT}}` (an upstream mirror - keep its tracked tree clean; work in
your own worktree).

> **Integration-project specs** (a separate Playwright project, e.g. `--project={{INTEGRATION_PROJECT}}`)
> need the whole integration stack up. Bring it up per **starting-the-e2e-backend** -> "Self-scoped
> integration project". The invocation is in "Integration project run" near the end of this skill.

## Per-worker parameters (compute first)

Compute `$slug`, `$be_port`, `$be_wt` exactly as in **starting-the-e2e-backend** ("Per-worker
parameters"). The Vite command below reuses `$be_port` to point the app at YOUR backend.

## Concurrency limit

Backends are fully parallel: each slug has its own DB, cache and port. The frontend layer is not:
Vite's port and Playwright's `baseURL` are hardcoded in tracked files. **Only ONE Vite + Playwright run
may be live per frontend port.**

- Before starting Vite: `curl -s -o /dev/null --max-time 3 {{FRONTEND_URL}}/login`. If something answers
  and you did not start it, another run is live: **never kill a run you did not start**; coordinate via
  `{{COORDINATOR_TELL}}` instead.
- Never start a second Vite on the default config: without `--strictPort` it silently binds the next
  port while both Playwright runs keep hitting the default one, so tests cross-wire instead of failing.

**Escape hatch (use it instead of killing an incumbent Vite).** The one-at-a-time limit comes from the
hardcoding, not a real constraint. Make the ports env-overridable with **uncommitted, throwaway** edits
(`vite.config.ts`: `server.port: Number(process.env.VITE_DEV_PORT) || {{FRONTEND_PORT}}`;
`playwright.config.ts`: `baseURL` / `webServer.url` from `process.env.PW_BASE_URL`), boot
`VITE_DEV_PORT=<alt-port> ... yarn start --strictPort`, and run with `PW_BASE_URL=http://localhost:<alt-port>`.
**Never commit those edits.** Prefer this over asking another worker to stop and over reaping a Vite
you did not start (state notes like "left up for reuse" go stale fast). If you do reap someone's
process, say so in your report.

## Quick reference

| Step | Command |
|------|---------|
| Run a spec | `cd <your frontend worktree> && {{DEVSHELL}} yarn playwright test <spec> --project=chromium` |
| Vendor-A-path specs (assert the distribution API got a request via the mock verifiers in `vendorAMock.ts`) | prefix `{{VENDOR_A_MOCK_URL_ENV}}=http://localhost:{{VENDOR_A_MOCK_PORT}}` AND boot the mock first (starting-the-e2e-backend -> "Vendor-A mock"). Without both, log purges no-op, log reads throw, and the backend's outbound call is connection-refused. |
| Filter by title | append `-g "<title substring>"` |
| Headed | append `--headed` (needs a display) |
| Check backend up | `curl -s -o /dev/null --max-time 3 http://localhost:$be_port/ && echo up` (readiness = curl *connects*; 200/302/404 all mean up) |
| Check Vite up | `curl -s -o /dev/null --max-time 3 {{FRONTEND_URL}}/login && echo up` |

## Preconditions (verify before running)

1. **Backend up and seedable** - per **starting-the-e2e-backend**.
2. **`{{E2E_ROOT}}.env`** exists in YOUR frontend worktree (gitignored):
   `BACKEND_URL=http://localhost:$be_port` (substitute the number), the admin credentials
   (`{{ADMIN_EMAIL}}` / `{{ADMIN_PASSWORD}}`).
3. Login and most specs do **not** touch external services or the vendor-A mock. Only vendor-A-path
   specs need the mock and its env.

> Editing a mock to make a spec pass? Do it via `editing-backend-seeds-and-mocks`: routes come from the vendor's own API docs, not imagination, and responses must be dynamic
> (echo request data, generated ids, computed dates).

## Bring up Vite (backend first)

Start the backend first, then Vite, in its own tmux session suffixed with your slug so workers don't
collide. **First check whether it is already up** (curl above); if it connects, skip bring-up. Only kill
a session carrying YOUR slug suffix.

```bash
tmux kill-session -t e2e-vite-$slug 2>/dev/null || true
tmux new-session -d -s e2e-vite-$slug -x 220 -y 50
tmux send-keys -t e2e-vite-$slug "cd <your frontend worktree> && {{SECRETS_WRAPPER}} {{DEVSHELL}} bash -c '{{BACKEND_URL_ENV}}=http://localhost:$be_port {{FRONTEND_ENV_VARS}} yarn start --strictPort' 2>&1 | tee /tmp/e2e-vite-$slug.log" Enter
```

- **Boot Vite with the exact env set CI uses** (the `env:` block of the CI Playwright workflow), not just
  `{{BACKEND_URL_ENV}}`. A missing one fails real specs in a way that looks like a product bug. If a spec
  fails locally but is green in CI, **diff your boot env against that `env:` block before reporting
  anything as a defect.** Secret-valued keys (e.g. `{{MAP_TOKEN_ENV}}`) arrive via `{{SECRETS_WRAPPER}}`
  and need no inline value.
- `--strictPort` makes Vite FAIL if the port is taken instead of silently moving on (which would
  cross-wire your tests into another worker's frontend).

**Wait for readiness without sleeping in your own shell.** Push the wait loop into the pane, let it
print a sentinel, and observe by capturing the pane:

```bash
tmux send-keys -t e2e-vite-$slug "until curl -s -o /dev/null --max-time 3 {{FRONTEND_URL}}/login; do sleep 2; done; echo VITE_READY" Enter
tmux capture-pane -p -t e2e-vite-$slug | grep -q VITE_READY && echo up
```

If the sentinel never appears, read `/tmp/e2e-vite-$slug.log`.

Then run the Quick-reference command. Playwright reuses the running Vite (`reuseExistingServer`); do not
rely on its built-in `webServer` to auto-start Vite.

## Confirm success

- Terminal shows `N passed`. On failure, report the output verbatim and where it broke (browser launch /
  backend / Vite / assertion). Do not paper over a failure.
- **Confirm the browser is the pinned one:** `{{DEVSHELL}} bash -c 'echo $PLAYWRIGHT_BROWSERS_PATH'` must
  point into the dev shell's store, NOT `~/.cache/ms-playwright`. A cache path (or empty) means you ran
  outside the dev shell.
- **Seed failures:** specs POST to `{{SEED_ENDPOINT}}` in `beforeEach`. A 404/500 there is a backend/seed
  problem, not a test bug - see "Seed endpoint" in **starting-the-e2e-backend**, and use
  **investigating-e2e-failures** to dig into a failed run.

## Teardown / leave as found

- **Backend:** tear down your OWN per-slug stack per **starting-the-e2e-backend** (`down`, never
  `reset`; then remove your backend worktree). There is no shared stack.
- **Vite:** kill only the session you started (`tmux kill-session -t e2e-vite-$slug`). Leave
  `{{E2E_ROOT}}.env` in place.

## Common mistakes

| Mistake | Fix |
|---------|-----|
| Running `yarn playwright test` outside the dev shell | Prefix `{{DEVSHELL}}`. The browser must resolve from the pinned shell, not `~/.cache`. |
| Setting `PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS` / `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD` / patchelf-ing a downloaded browser | Not needed with the pinned browser. If you feel you need them, you are on the wrong path. |
| Starting a second Vite when the port is busy | One Vite per port. `--strictPort` guards the silent-next-port trap; use the escape hatch or coordinate, never kill the other run. |
| Booting Vite with only the backend URL env | Use CI's full `env:` block; a missing var makes specs fail like product bugs. |
| Committing run artifacts to the mirror | `{{FRONTEND_CHECKOUT}}` stays pristine. `{{E2E_ROOT}}.env` is gitignored; never commit e2e output. |
| Backend won't come up or won't seed | Backend layer - see **starting-the-e2e-backend**. |
| `yarn lint` from a worktree nested under `.worktrees/` fails `all-files-ignored` | Not a real lint failure. Without `root: true` in the ESLint config, the config cascade walks up to the mirror root, every worktree file resolves under `.worktrees/...`, and ESLint's built-in dot-dir ignore swallows them. Fix with zero file edits: `yarn lint --no-ignore` (re-add any real ignores with `--ignore-pattern`). Do not add `root: true` (dirties a tracked file); do not lint the mirror (it won't see your edits). Or use a non-nested sibling worktree. |

## Integration project run

The integration project (`{{E2E_ROOT}}tests/integration/`) is registered in `playwright.config.ts`
and selected only with `--project={{INTEGRATION_PROJECT}}`; other projects pin their own `testDir`, so a
bare run never sweeps these in. Bring the **stack** up per **starting-the-e2e-backend** -> "Self-scoped
integration project"; this section is Vite + invocation.

- **Vite:** boot from a **non-nested sibling** frontend worktree (avoids the nested-worktree ESLint trap
  in the dev-server checker with zero file edits). Symlink `node_modules` from the mirror when the
  lockfile matches. Point it at the integration backend's port.
- **`{{E2E_ROOT}}.env`:** `BACKEND_URL`, admin creds, `{{VENDOR_B_MOCK_URL_ENV}}`, `{{VENDOR_A_MOCK_URL_ENV}}`.
- **Use `127.0.0.1`, not `localhost`, in every URL the Node/Playwright side uses.** A mock that binds
  IPv4 only is unreachable when Node resolves `localhost` to `::1` first (`ECONNREFUSED ::1:<port>` in
  the pre-test purge fixture).

Run in the dev shell, from a tmux pane in the same network namespace as the stack (see the backend
skill; the agent's own shell may live in a different one):

```bash
cd <sibling frontend worktree>
{{SECRETS_WRAPPER}} {{DEVSHELL}} env \
  BACKEND_URL=http://127.0.0.1:<integration-be-port> \
  {{VENDOR_B_MOCK_URL_ENV}}=http://127.0.0.1:{{VENDOR_B_MOCK_PORT}} \
  {{VENDOR_A_MOCK_URL_ENV}}=http://127.0.0.1:{{VENDOR_A_MOCK_PORT}} \
  FRONTEND_URL=http://127.0.0.1:{{FRONTEND_PORT}} \
  yarn playwright test --project={{INTEGRATION_PROJECT}} --reporter=line <spec>
```

- **Run each flow group in a SEPARATE invocation** (e.g. outbound-request specs vs. inbound-import specs
  that need object storage). A mixed single-worker run accumulates gateway-side state and timing load,
  producing harness flakes that are not product bugs.
- When a run does not need the vendor-A mock, leave `{{VENDOR_A_MOCK_URL_ENV}}` **unset** unless the mock
  is actually up: a global setup that purges it unconditionally fails on ECONNREFUSED.

## Maintenance coupling

When the repo bumps `@playwright/test`, bump the dev shell's pinned browser input in lockstep so the
browser revision still matches. A mismatch fails browser launch.
