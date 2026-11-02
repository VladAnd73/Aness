# Configuration

Optional `spore.toml` and NixOS module settings. The README covers the
defaults; read this when you need more than one worker agent, an
external work source, or deploy-time draining.

## Matter plugins

Matter plugins pull work from external sources (Linear, GitHub
Issues, ad-hoc adapters) and mirror local task transitions back to
them. Configure one or more in `spore.toml` under
`[matter.<name>]`, or via `SPORE_MATTER_<NAME>__<KEY>` env vars
when the NixOS module is the source of truth (the loader merges
both).

The fleet reconciler runs `Sync` against every enabled matter
before it enumerates tasks, so new upstream tickets land as
`tasks/<slug>.md` files automatically on the next pass. When a
task carrying matter metadata flips to done, an `OnDone` hook
mirrors the close back upstream; a fallback sweep on the next
`Sync` covers misses (reconciler off, adapter down, task edited
out of band).

Tasks created by a matter carry three frontmatter keys: `matter`
(adapter name), `matter_id` (upstream id), and `matter_url`
(deep link, optional).

### Linear

The bundled `linear` adapter polls a configured team for issues in
a ready state, projects each one to `tasks/<slug>.md`, pushes the
issue to in-progress, and mirrors `status: done` flips back to a
configured done state.

```toml
# spore.toml
[matter.linear]
enabled = true
team = "MAR"                       # team key, required
ready_state = "Ready"              # default
in_progress_state = "In Progress"  # default
done_state = "Done"                # default
api_key_env = "LINEAR_API_KEY"
# or, when the NixOS module supplies the secret via systemd:
# api_key_file = "linear-api-key"
```

For NixOS deployments, configure the same shape under
`services.spore-fleet.matters.linear`. Secrets ride systemd
`LoadCredential` and never enter Nix evaluation or `/nix/store`.
See [nixosModules/spore-fleet.nix](../nixosModules/spore-fleet.nix).

## Worker mix

The fleet reconciler picks a worker agent (Claude, Codex, or any
binary on PATH) for each task it spawns. Default behaviour without
`spore.toml` is "all Claude". To run a mix, declare a default,
optionally a ratio, and optionally rules keyed on a task's
`complexity:` extra:

```toml
# spore.toml
[fleet.workers]
default = "claude"

[fleet.workers.ratio]
claude = 70
codex = 30

[fleet.workers.rules]
mechanical = "codex"
deep = "claude"
```

Selection precedence:

1. An explicit `agent:` already in the task frontmatter wins.
2. A rule whose key matches the task's `complexity:` value wins next.
3. Otherwise the ratio balancer picks the agent whose currently
   spawned share is furthest below its target share, so the active
   fleet converges on the configured split.
4. Otherwise `default` (or `claude` when omitted) is used.

The picked agent is written back into the task's frontmatter
`agent:` when reconcile spawns the session, so downstream tools see
the same answer the launcher used. Coordinator agent selection lives
under the separate `[coordinator]` section and is never subject to
the worker ratio.

## Coordinator context caps

The coordinator token monitor (a Stop hook) asks the coordinator for a
handoff at the soft cap and for a respawn at the hard cap. Both count
context tokens. Defaults are 350000 and 400000.

```toml
# spore.toml
[coordinator]
soft_cap = 350000
hard_cap = 400000
```

`SPORE_COORDINATOR_SOFT_CAP` and `SPORE_COORDINATOR_HARD_CAP` win over
the file. Put them in the project's `agent.env` (see
`docs/agent-env.md`) to reach the coordinator session. If the result
puts soft at or above hard (for example only `hard_cap` is set below
the default soft cap), the soft cap drops to 7/8 of the hard cap so
the handoff warning still comes first.

The monitor finds `spore.toml` through `SPORE_PROJECT_ROOT`, then
`CLAUDE_PROJECT_DIR`, then the working directory. Projects that still
run their own bash `bin/dev/spore-token-monitor` ignore these settings;
see `docs/todo/retire-bash-token-monitor.md`.

## Graceful deployment

When the NixOS module is enabled with `gracefulDeploy.enable =
true` (the default), pre and post activation hooks drain active
workers around `nixos-rebuild switch` and colmena deploys:

- pre-activation disables the kill-switch, drops a wrap-up
  message into every active worker's inbox, waits up to
  `gracefulDeploy.timeout` seconds for them to flush, then kills
  any holdouts;
- post-activation re-enables the kill-switch so the next
  reconcile pass repopulates the fleet.

The `preScript` and `postScript` paths are also exposed as
read-only options so a colmena `deployment.preActivation` /
`postActivation` hook can drive the same drain remotely. Set
`gracefulDeploy.enable = false` for one-off worker tiers whose
tasks should never see a wrap-up signal.

## Agent session shell

spore starts coordinator and worker sessions as
`tmux new-session ... <bash> -c '<command>'`, with `bash` looked up on
`PATH` (falling back to `/bin/sh`). The tokenmonitor's
`respawn-pane` runs `/bin/sh -c '<command>'` the same way. tmux execs
that argv directly. Before this, tmux ran the command string through
the server's `default-shell`, which it takes from `$SHELL` when the
server starts. On a spore host that was the `spore-attach` login shell,
and every spawn died.

What changed for a downstream project:

- `SPORE_AGENT_BINARY`, `SPORE_COORDINATOR_AGENT` and the spawn
  command are parsed by bash, not by your `default-shell`. Write them
  in POSIX/bash syntax. fish- or zsh-only syntax fails.
- Files your own shell reads for every `-c` run, such as zsh's
  `~/.zshenv`, no longer apply to agent sessions. Put the environment
  an agent needs in the project's `agent.env` (see
  [agent-env.md](agent-env.md)) or the systemd unit instead.
- tmux's `default-command` option does not apply to these sessions.
- A tmux server that spore itself starts gets `SHELL=<bash>`, so its
  `default-shell` is bash. Servers started elsewhere keep theirs.
