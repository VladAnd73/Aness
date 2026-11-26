# Changelog

## Unreleased

- Three new bundled skills land in every project on the next
  `spore install`, adapted from mattpocock/skills (MIT):
  `reviewing-against-the-brief` (a two-pass review: the diff against
  the `tasks/<slug>.md` acceptance scenarios, then against the
  project's written rules that no lint covers),
  `running-a-harness-retro` (operator-invoked; proposes rule, lint,
  hook and skill fixes after a session and leaves memories to the
  dream run) and `building-a-repro-loop` (build a fast failing check
  before theorizing about a flaky, CI-only or cannot-reproduce bug).

## 1.0.12 - 2026-11-20

- `spore research` improves one skill overnight: an editing agent
  proposes one edit per try, a locked and sandboxed judge scores it,
  and only edits confirmed better on two judgings are kept. Nights are
  started by hand, capped by tries, hours and live weekly quota, and
  need `spore budget room`. `stop` runs a sealed holdout set and tells
  the coordinator; `accept` lands the winner (a `research/<campaign>`
  branch for a skill in the spore repo). See `docs/autoresearch.md`.
- `spore eval judge` scores a candidate `SKILL.md` on a folder of data
  scenarios (JSON prompts, fixture files, fake MCP services, check
  kinds), with repeats, bubblewrap isolation and a JSON score.
- `spore research new --kind role --step NAME` targets a loop role body
  (any `.md`), pinning the step's output, shape and skills from the
  project's loop file. Its judge (`spore eval judge --kind role`) replays
  known bugs: the agent gets the feature spec, the pre-fix code snapshot
  and keyed fake Linear and Notion services that serve only allowed
  ids; a grader that alone sees the repro steps must quote the line that
  catches the bug. `fluff` caps a plan's scenario count and
  `code-quality` fails added comments, visibility-only tests, duplicated
  helpers and API end-state asserts. `judge draft` on a role campaign
  mints the corpus-builder brief.
- Under `maximize-score` the holdout winner must now beat the
  original, not tie it (`shrink-holding-score` keeps "at least as
  well"); an original at 100 is `no-headroom`, which `accept` refuses without
  `--force`. The holdout cache is keyed by the judge (holdout set,
  binary, command). Under `maximize-score` a night refuses with
  `no-headroom` when the baseline already scores 100 on the main set
  (a new `judge approve` restarts the campaign), and a kept try that
  reaches 100 ends the night with `max-score` and runs the holdout.
- `accept` commits a role winner on `research/<campaign>` in the role's
  own git checkout; skills keep the spore-repo-only branch rule.
- The bubblewrap sandbox is NixOS-only for now: `research new`, `night`
  and `eval judge` refuse `isolation bwrap` elsewhere and name
  `--isolation none` (`research new` gained the flag); tests that run
  the real sandbox skip off NixOS.

## 1.0.10 - 2026-11-17

- `push-and-open-pr` and the role-loop PR mirror work on an ssh
  origin. A `git@github.com:o/r.git` or `ssh://git@github.com/o/r.git`
  origin is pushed to `https://github.com/o/r.git` by URL, which the
  GitHub App token can authenticate, without touching any remote
  config. `remote.origin.pushurl` is honored before `url` (a fork
  workflow pushes to the fork, and gh is pinned there). When origin
  carries several urls, the first https one wins, so an extra ssh url
  is never tried. The github.com host matches in any case, and
  `git@github.com:/o/r.git` is accepted. An ssh origin on another host
  or an ssh config alias fails with an error naming the host and
  saying to set an https `remote.origin.pushurl`. The app push never
  leaves over ssh: a `url.<base>.insteadOf` or `pushInsteadOf` rule
  that rewrites the https URL to ssh is refused.
- Loop state under `.spore/` is git-excluded. Spore adds `.spore/` to
  the repo's `info/exclude` (in the common git dir, so a linked
  worktree updates the main repo's) when it first creates `.spore/`.
  Loop files no longer show as untracked and trip a
  `git status --porcelain` done check. The tracked `.gitignore` is not
  touched. Only a project root that is the top of its own repo is
  excluded (a plain dir inside a parent repo, or an exported
  `GIT_DIR`, never writes another repo's exclude), and an exclude
  spore cannot write is logged without blocking a drive or a spawn.
- Loop files take `pr_footer = false` to drop the "Dispatched by ...
  merges." footer from PR bodies opened by `push-and-open-pr` or the
  draft-PR mirror. Absent, the footer stays.
- `summary = true` on a custom loop writes `summary.md` from that
  loop's own agent steps (one section per step, each round's output)
  instead of an empty dev-loop review summary. Only the loop named
  `dev` gets the dev review summary. The summary is rewritten when an
  output's content changes, not only its round count. The dev loop's
  summary is unchanged.

## 1.0.9 - 2026-11-17

- `spore gh config` and `spore gh token` refuse to act when `SPORE_PROJECT_ROOT` names one repo and
  the cwd is inside another, naming both roots. Before, it silently
  read or wrote the env project's `github.toml`. A new `--project
  <path>` flag on `config`, `token` and `exec` picks the project
  explicitly. A worktree of the same project, or a cwd outside any git
  repo, still resolves to `SPORE_PROJECT_ROOT`. `spore gh exec` never
  refuses on a mismatch: its cwd is often a throwaway https clone of
  another repo, as in the bot PR recipe. `--project` is made absolute
  (`--project .` is the cwd's repo, not the shared state root) and
  must name a path inside a git repo, so a typo is refused instead of
  creating a stray `github.toml`.

## 1.0.7 - 2026-11-17

- Configurable task loops. A task with `loop: <name>` in its
  frontmatter is driven through the steps of
  `bootstrap/loops/<name>.toml` on every fleet reconcile: each `agent`
  step runs in its own pane with its own role body, `action` steps run
  kernel actions (`notify-coordinator`, `push-and-open-pr`), and `next`
  entries route on the step's verdict, including back to an earlier
  step with `return = true` and a `max_rounds` cap that escalates.
  Done conditions can be a file, a verdict or a `command` check run in
  the task worktree. Two loops ship: `dev` (the upstream engineer,
  reviewer A, reviewer B loop; `loop: role` still means it) and a
  small `example`. A project file of the same name overrides a bundled
  one. Ported from upstream's role loop. See `docs/role-loop.md` and
  `docs/todo/configurable-loops.md`.
  - Each step pane's first prompt starts with a note (`SPORE_STEP_NOTE`)
    naming its output path, round, keys and author, overriding the
    role body; wakes name the round to write too.
  - `spore task status <slug>` prints the task's own loop, step, round
    and state; `spore task role-drive <slug>` runs one drive tick;
    `spore task new --role` writes `loop: role`.
  - A failed command check reruns only when the step output or the
    task branch changes, never counts toward `max_rounds`, and a newer
    round replaces a held one. Held steps are reported once.
  - Reconcile reaps the loop panes of tasks that are done, deleted or
    in any status but active, paused or blocked, and never drives one
    slug twice at once.
  - Role panes are metered by the worker token monitor, and
    `SPORE_CODEX_BIN` picks the codex binary for codex panes.
- `spore task done` removes `.spore/<slug>/`, so a reused slug starts
  its loop fresh.
- Every codex worker and role pane launch passes the CLAUDE.md
  project-doc fallback, so they get the project's instructions in a
  repo without AGENTS.md. A codex coordinator does not get it yet.
- `[fleet] pre_start` now runs for plain workers too, and with
  `[worker] isolate_network` it runs inside the network namespace with
  the agent, so the agent reaches the services it starts.

## 1.0.6 - 2026-11-16

- `spore gh exec -- gh pr create ...` assigns the PR to
  `operator_login` from `github.toml` when the command names no
  assignee. A failed assign only warns.

## 1.0.3 - 2026-11-11

- `spore gh config|token|exec` acts on GitHub as the project's GitHub
  App: it mints and caches installation tokens from a host-side
  `github.toml`, and `exec` runs a command with `GH_TOKEN` and a git
  credential helper so `gh` and `git push` over https act as the app.
  Ported from the upstream team branch. See `docs/github-app.md` and
  `spore recipes show github`.

## 1.0.2 - 2026-11-09

- `spore budget daily` records once a day how much of the weekly
  Claude quota is used against how much of the week has passed, and
  `spore budget room` answers room / no-room / unknown by exit code.
  Any failed run leaves `room` answering unknown. A user timer
  template (`spore-budget-daily.timer`, 18:00 UTC) ships in
  `bootstrap/handover/systemd/` and the shims package; install pins
  its state dir, config and credentials from the new
  `spore budget timer-env`. See
  `docs/budget.md`, "Daily headroom".
- `spore budget refresh`, `summary` and the advice band no longer read
  a missing or null weekly (`seven_day`) usage as 0%: the snapshot
  goes stale instead, and a store account with such a body is left out
  of the average.

## 1.0.1 - 2026-11-09

- Worker TDD rule: an implementation plan's first task writes the whole
  acceptance suite and proves it red before any implementation, and a
  brief for a code-changing worker names
  `superpowers:test-driven-development`.

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
