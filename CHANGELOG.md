# Changelog

## Unreleased

## 1.0.0

- Every merge to `main` now cuts a version. The new Release workflow
  (`.github/workflows/release.yml`) runs `scripts/bump-version`, which
  bumps the patch number in `VERSION`, moves these `## Unreleased`
  notes under `## X.Y.Z - YYYY-MM-DD`, commits `release: vX.Y.Z` to
  `main` and pushes tag `vX.Y.Z`. A PR that sets `VERSION` itself (a
  minor or major bump) is tagged as is, with no bump commit; rename the
  `## Unreleased` heading in that same PR. `just release` is gone.
- Version numbering restarts at 1.0.0 on the fork. Upstream
  `marketertechnologies/spore` already owns tags `v0.4.4` to `v0.29.1`,
  so the fork's next patch would have clashed with them. The flake's
  package version jumps from `0.4.3` to `1.0.0`; a downstream flake
  that pins `spore` by tag should move to `v1.0.0` or later.
- The GitHub repo moved from `VladAnd73/AgenticHarness` to
  `VladAnd73/Aness` (2026-11-03). Every repo URL now points at the new
  name: the bundled `bootstrap/flake` `spore` input and its lock entry,
  the `spore infect` pin and push-first guard, the handover systemd
  units, the README badges and install line, and CI's Codecov slug. The
  Go module path stays `github.com/versality/spore`. GitHub redirects
  the old name for now, but a downstream flake that pins
  `github:VladAnd73/AgenticHarness` should move its `spore` input to
  `github:VladAnd73/Aness`.
- NixOS module: `spore migrate` now runs during `nixos-rebuild switch`
  with bash, coreutils, findutils, grep, sed, git and spore on PATH.
  Before, every migration failed there with `exec: "bash": executable
  file not found in $PATH`. Run `spore migrate` by hand once to apply
  any migration a past rebuild skipped.
- Handover settings: a no-op PreToolUse hook now matches
  `AskUserQuestion`. With it, Claude Code writes a pending question to
  the session transcript before the answer arrives, so a console that
  reads the transcript can show the full question. Hosts pick it up on
  the next handover settings install.
- Token caps: the coordinator token monitor now asks for a handoff at
  350000 context tokens and a respawn at 400000 (was 210000 / 250000).
  Workers wrap up at the same hard cap (was 180000, or 240000 with
  `SPORE_ACCOUNT_TIER=max`). Set the caps with `[coordinator] soft_cap`
  / `hard_cap` in `spore.toml` or `SPORE_COORDINATOR_SOFT_CAP` /
  `SPORE_COORDINATOR_HARD_CAP`; `SPORE_WORKER_TOKEN_WRAP` still
  overrides the worker cap alone. `SPORE_ACCOUNT_TIER`,
  `SPORE_WORKER_TOKEN_WRAP_MAX` and `SPORE_WORKER_TOKEN_WRAP_SUB` are
  removed and now ignored: if you set them, move the value to
  `SPORE_WORKER_TOKEN_WRAP`. The new defaults assume a 1M context
  window; on a 200K setup set `hard_cap` below about 190000. Projects
  still on the bash `bin/dev/spore-token-monitor` are unaffected. See
  `docs/configuration.md`.
- `flake.nix`: Claude Code's version is now an explicit pin. A second
  nixpkgs input, `nixpkgs-claude-code`, is held at one named revision
  and read for exactly one package, exposed as
  `packages.<system>.claude-code`. Before this, a host took Claude Code
  from whatever nixpkgs revision its own system flake sat on, so moving
  Claude Code meant moving the kernel, systemd and every other package
  in the same rebuild. The pin currently resolves to 2.1.296. It
  replaces the `sadjow/claude-code-nix` input, which had not been
  re-locked since August; both package the same official release binary
  with the same `$out/bin` layout. The pin is not the cheaper of the
  two: it carries a second nixpkgs of its own, measured at 209.9 MB
  against sadjow's 211.7 MB. What changes is who chooses the revision
  and how much lock the choice costs: three nodes leave spore's lock
  (`claude-code` and its own `nixpkgs` and `systems`) and one arrives,
  and the Claude Code version becomes something a spore reviewer picks
  instead of something a third-party packaging repository picks. A new
  flake check, `claude-code-version`, runs the
  built binary and fails below the floor. The root `nixpkgs` revision is
  unchanged: it only loses the `_2` suffix in `flake.lock` because the
  node that held the plain name is gone. See `docs/claude-code-pin.md`
  for adopting the pin on a host and for bumping it.
- `bootstrap/flake/configuration.nix`: the shipped template takes
  claude-code from spore through a single binding used for both the
  interactive `claude` on PATH and `services.spore-fleet.claudeCodePackage`,
  so the two cannot drift apart unnoticed. Downstream consumers inherit
  the pin when they re-lock their spore input.
- `internal/agentenv`: new per-project environment file. spore now
  reads `${XDG_CONFIG_HOME:-$HOME/.config}/spore/<project>/agent.env`
  and seeds its `KEY=VALUE` entries into every tmux session it spawns,
  for both the coordinator and workers. Lets one project's agents run
  with a different account, budget state directory or endpoint than the
  rest of the host, without touching any systemd unit. A project
  without the file behaves exactly as before. Nothing expands and
  nothing unescapes; matched surrounding quotes are stripped and an
  unmatched one is an error, so the file cannot quietly produce a
  value that only half-works. A malformed line refuses the spawn, a
  `SPORE_*` / `WT_*` key is dropped with a message, and a mode looser
  than 0600 warns. No message ever contains a value, only the key and
  the line number. See `docs/agent-env.md`.
- `bootstrap/handover/spore-coordinator-launch.sh`: support a host-wide
  shared role file in addition to the per-project `$SPORE_COORDINATOR_ROLE`.
  The launcher now reads `$SPORE_COORDINATOR_ROLE_SHARED` (default
  `${XDG_CONFIG_HOME:-$HOME/.config}/spore/coordinator-role.md`) and
  concatenates the shared file and the project role file with a blank
  line between them before passing the result as the agent's first user
  message. Each part is optional; missing parts are skipped, and when
  both are missing the agent launches unseeded as before. Operators can
  factor common respawn / wrap-up / operating-regime boilerplate out of
  per-project role files and keep only project-specific identity and
  deltas in each `bootstrap/coordinator/role.md`. Backward compatible:
  hosts without a shared file behave exactly as today.
- `nixosModules/spore-fleet`: replaced single-project
  `services.spore-fleet.projectRoot` with the multi-project
  `services.spore-fleet.projects` attrset
  (`name → { path }`). The module now generates one
  `spore-fleet-reconcile-<name>` service + timer + path watchers
  per entry, so a single host can reconcile several project trees
  under the same `user` without external glue. Tmux session naming
  (`spore/<name>/coordinator`, `spore/<name>/<slug>`) is already
  namespaced by project, so coordinator and worker sessions stay
  isolated.

  `projectRoot` is kept as a deprecated alias: when set (and
  `projects` is empty), it surfaces as
  `projects.${baseNameOf projectRoot}.path`, with a deprecation
  warning. Setting both is an assertion error; setting neither
  with `enable = true` is also an assertion error.

  The kill-switch flag at `~/.local/state/spore/fleet-enabled`
  remains host-wide; flipping it triggers reconciles across every
  project. The pre/post graceful-deploy scripts now drain workers
  for every configured project sequentially.
- `matter.linear`: Sync now projects new Linear comments per active
  ticket as tell-envelope JSON files in the matching rover slug's
  spore inbox dir, so an operator commenting on a Linear ticket
  reaches the rover working it via the existing watch-inbox Stop
  hook. Per-ticket cursor lives at
  `<StateDirForProject>/<slug>/linear-comments.cursor`; first
  observation seeds at "now" so historical comments do not flood
  the inbox.
- Renamed the per-task inbox env var from `SKYBOT_INBOX` to
  `SPORE_TASK_INBOX` to drop a foreign prefix that had leaked in
  from a consumer harness. Spore sets and reads the new name only;
  consumers that exported `SKYBOT_INBOX` for operator-side hooks
  must rename on their side. No backward-compat alias.

## 0.4.2 - 2026-05-06

- Fixed `spore coordinator start` lying about success when the agent
  binary failed to exec. `driverToBinary("claude")` returned the
  package name `claude-code` instead of the actual binary name
  `claude`, so the inner shell command died on `exec` and tmux tore
  the session down before any `has-session` check ever ran. Two
  changes: (1) "claude" now maps to `claude` (the kernel default
  fallback also moves to `claude`); (2) `EnsureCoordinator` now
  waits a short settle window after `tmux new-session -d` and
  re-checks the session, returning a real error if the spawn died.
- Synced `VERSION` with the git tag scheme (was stuck at `0.0.3`
  while tags advanced to `v0.4.x`). New `just release X.Y.Z` recipe
  validates clean+green, bumps `VERSION`, commits, and tags so the
  two stay in step from now on.
- Refactored `embed_test.go` to read `VERSION` at test time instead
  of hardcoding the expected string, so a release no longer requires
  a paired test edit.

## 0.0.3 - 2026-05-05

Spore 0.0.3 lands the universal coordinator entry point. "How do I
start the coordinator?" now has the same answer for every consumer:
`spore coordinator start`.

- Added `spore coordinator start [--wait] [--poll-sec N]`,
  `stop`, `restart`, and `status` over the existing fleet
  coordinator helpers. `start` is idempotent; `--wait` blocks
  until the session exits (helm-spawn / skyhelm-spawn shape).
- Added the `[coordinator]` section in `spore.toml` with `driver`,
  `model`, and `brief` keys. Env vars still win; driver "claude"
  maps to `claude-code`, "codex" to `codex`, and any other value
  passes through (so a project can point at a launcher script).
- Injected `SPORE_COORDINATOR_PROVIDER` and
  `SPORE_COORDINATOR_MODEL` into the session env when configured,
  matching the bundled `spore-coordinator-launch` dispatch shape.
- `status` prints up/down, the configured driver/model/brief, and
  exits 3 when the coordinator session is down (0 when up).

## 0.0.2 - 2026-05-04

Spore 0.0.2 is a focused release: it can now run Codex-backed workers
as a first-class task option while keeping the
existing Claude Code path intact. Feedback and rough edges are welcome.

- Added Codex worker support through `agent: codex`, with task-level
  model and reasoning-effort selection.
- Kept mixed Claude Code and Codex workflows on the same task
  frontmatter, tmux worker sessions, inbox/tell protocol, and merge
  close path.
- Bumped the package and CLI version to `0.0.2` and added
  `spore version`.
- Smoothed the README opening flow by removing the "Choose a Path"
  table.
