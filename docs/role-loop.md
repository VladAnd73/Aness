# Role-based fleet: operator guide

## Spawn hardening in this fork

Role panes get the same spawn treatment as generic workers. Their
tmux session carries the project's `agent.env` entries after spore's
own `-e` flags (reserved `SPORE_*` and `WT_*` keys are dropped, as for
workers), and with `[worker] isolate_network = true` the agent runs
under `pasta`; a missing `pasta` refuses the spawn instead of running
the pane unisolated. A pane that dies within ten minutes of its spawn
waits out a backoff (1 minute, doubling to 30) before the next spawn.
The backoff is kept per pane, as `<slug>/<role>` (for example
`demo/engineer`), in `role-spawns.json` in the project state dir,
separate from the generic fleet's `fleet-spawns.json`. A pane that
stays healthy past ten minutes clears its record. The backoff applies
to both the silent respawn of a missing session and the counted
respawn of a dead agent; the `MaxRoleRespawns` cap is unchanged. Role
notices go to the coordinator inbox the watcher reads, resolved from
the project root and the state dir settings.

Role panes have no `SPORE_TASK_INBOX`, so `spore worker token-monitor`
does not meter them: a role agent gets no wrap reminder and runs until
the agent compacts on its own. If a token wrap or anything else kills
a role session, the driver respawns it silently on the next tick.
