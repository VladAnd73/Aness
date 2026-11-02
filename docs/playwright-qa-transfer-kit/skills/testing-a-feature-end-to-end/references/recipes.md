# Infra recipes

Concrete recipes used across the pipeline. They support the stages in `pipeline-lessons.md`; the granular
runner/backend skills own the full how-to - this file is the quick reference for the glue.

## Per-slug isolated backend (stage 4)

- Every consumer provisions its OWN backend - there is NO shared stack. Skill: `starting-the-e2e-backend`
  (branch-aware mode, per-slug worktree `{{BACKEND_WORKTREE_ROOT}}-{{SLUG}}`, own DB/cache/hashed port).
- Use a **SHORT** task slug. A long slug pushes the Postgres unix-socket path past 107 bytes -> the backend
  will not boot (the services wrapper fails to bring services up).
- Prepare the test DB with `db:schema:load` (NOT migrate); CI's backend artifact does the same via
  `db:test:prepare`. Verify the expected schema state (e.g. presence/absence of a column) before running.
- `{{SERVICES_WRAPPER}}` in `{{BACKEND_CHECKOUT}}` keys off the task slug; never let it default to the
  coordinator's slug - pass an explicit short throwaway slug.

## Admin-panel e2e needs the review-app flag (stages 4, 7)

- Admin-panel Playwright specs need the backend booted with `{{REVIEW_APP_FLAG}}` set - the passwordless
  admin sign-in route is gated on it. Without it, the admin sign-in helper 404s and ALL admin specs fail
  (every page redirects). Any value works.
- Consequence for bug hunting: access DENIAL is not UI-testable in e2e - the same review-app backend
  auto-signs-in an admin when admin authentication fails, so denial coverage belongs in backend specs, not
  Playwright.

## Vendor mock - {{MOCK_TOOL}} on {{VENDOR_A_MOCK_PORT}} (stages 4, 6)

- Asserts on outbound {{VENDOR_A}} calls need {{MOCK_TOOL}} on `{{VENDOR_A_MOCK_PORT}}` serving the vendor-A
  data file from `{{MOCKS_HOME}}`. CI boots it automatically; local runs must start it (pin the mock CLI
  version CI uses).
- Assert the outbound request via the reusable transaction-log helper (reference project:
  `playwright/support/vendorAMock.ts`, e.g. `verifyVendorReceivedRepublish`, matched by the item's NAME, not a
  shared uuid).
- The mocks repo is shared - changing a shared mock route is an operator-gated delta. For local proof, run
  your own mock copy and point your own backend's outbound URL env (`{{VENDOR_A_OUTBOUND_URL_ENV}}`, or the
  equivalent for any other external service) at it (zero shared edits).
- An ungated live external dependency is a known flake source: if the backend's test env points an
  outbound URL at a real hosted service and the code path RAISES on failure, CI (which cannot reach it) reds
  deterministically. Mocking it is the real fix; surface it as an operator gate.

## Push to a working branch (stage 9)

- Push under the secrets wrapper, inside the dev shell (a pre-push hook typically runs lint-staged and
  aborts without the toolchain on PATH):
  ```
  {{SECRETS_WRAPPER}} {{DEVSHELL}} --command git push <frontend-remote> HEAD:<branch>
  ```
- Commit authorship via per-command overrides (never `git config user.*`, which leaks into shared config):
  ```
  git -c user.name='<name>' -c user.email='<email>' commit ...
  ```
  using `{{AUTHOR_IDENTITY}}`.
- Keep the PR **DRAFT**; never merge; never retarget base. Verify the remote: `git ls-remote` tip == pushed
  SHA + `gh pr view` (wrap `gh` in `{{SECRETS_WRAPPER}}` if it is only authenticated there).
- Tokens rotate - check `gh auth status` under the wrapper before relying on it. A package-registry token
  for `yarn install` goes in the USER-level npmrc (`npm config set --location user`), NEVER
  `--location project` (that writes the token into the tracked `.npmrc`).
- Locale gotcha: a production build can rewrite tracked compiled `{{I18N_LIBRARY}}` catalogs
  (`<compiled catalogs path>`) - restore them with `git checkout --` after any build; never commit them.

## CI triage (stage 7)

- Frontend e2e CI runs `{{CI_BACKEND_ARTIFACT}}` (built from the backend's integration branch, NOT the PR's
  backend). A broken backend integration branch fails ALL PRs' e2e. Triage by diffing the PR's product
  source against the failing specs, and comparing the backend's schema file against its migrations for
  column drift (schema loads via `db:schema:load`, so a missing column -> 500).
- PR e2e failure runbook: kick ONE CI retry per SHA (`gh run rerun <run-id> --failed`) AND dispatch a
  local-repro worker in PARALLEL; green = silent, red again = escalate.

## Dispatching an idle worker (coordinator, cross-cutting)

- `spore task tell <slug> "<msg>"` wakes an idle worker. If a `tell` drains without re-prompting, or a
  dispatch sits UNSENT in the pane (the TUI swallows a bare Enter), use the send-keys recipe:
  ```
  tmux send-keys -t <session> C-u
  tmux send-keys -t <session> -l '<full prompt text>'
  tmux send-keys -t <session> Enter
  ```
