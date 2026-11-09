# Daily usage headroom

**Status**: design reviewed 2026-11-09, awaiting operator approval; not built.

## Intent

The operator pays for a Claude subscription with a weekly quota and
wants to use as much of it as possible without running out. Today
nothing on the host knows how close the account is to its limits, so
nothing can decide to do more work when there is room.

This feature only measures. Once a day it records how much of the
weekly quota is used, compares that to how much of the week has passed,
and writes a verdict: room or no room, and how much. A later, separate
feature will read that verdict and decide whether to start extra work.
How that later feature spends the room is its business, not this one's.

## The rule

The weekly window does not start on Monday. It starts 7 days before the
weekly `resets_at` the account reports. At any moment:

- `elapsed` = fraction of the 7-day window that has passed (0 to 1).
- `pace` = `elapsed` as a percentage. Two days into the week, pace is
  2/7, about 29%.
- `used` = weekly utilization percentage the account reports.
- `headroom` = `pace - used`, in percentage points. Positive means under
  pace.
- verdict `room` when `headroom >= margin`, else `no-room`.

`margin` comes from the config file (below). Example: on day 2, pace 29,
used 15, margin 10: headroom 14, verdict `room`.

## Goals

- One reading per day at 18:00 UTC. The verdict stands until the next
  18:00 reading. No continuous polling.
- The reading is always fresh. A stale or failed reading never produces
  `room`.
- One stable interface for consumers: a `spore budget` subcommand whose
  exit code is the verdict, so the later feature never parses files.
- A daily history so the week can be reviewed or charted later.

## Non-goals

- Reducing or throttling usage. This feature does not stop anything.
- Starting extra work. That is the later feature.
- Per-model pacing (for example the separate Fable weekly limit). Record
  those numbers in history if present, but the verdict uses the overall
  weekly limit only.
- The 5-hour window drives no verdict. Record it in history only.

## Anti-goals (failure even if it "works")

- Reporting `room` from old or missing data. Unknown must read as no.
- The daily probe session behaving like a fleet agent: running spore
  hooks, waking or draining a coordinator inbox, showing up in
  `spore task ls`, or being respawned by reconcile.
- Touching the operator's real `~/.claude/settings.json` status line.
- A design that needs a long-running interactive session to exist.

## How the reading is taken (decided)

**Primary: the existing `spore budget` usage endpoint.** `spore budget
refresh` already calls the OAuth usage endpoint (see `docs/budget.md`,
"Subscription mode") with the host's Claude login. Verified live
2026-11-09: it returned `seven_day.utilization` 22 with `resets_at`
2026-11-10T06:00Z and `five_hour.utilization` 3, costing no tokens and
needing no session. The endpoint is undocumented by Anthropic, so it
can change without notice. Treat any HTTP failure, a `Stale` snapshot,
or a missing `seven_day` block as a failed primary read.

**Fallback: a probe session reading the documented status line.**
Claude Code passes `rate_limits.five_hour` and `rate_limits.seven_day`,
each `{used_percentage, resets_at (epoch seconds)}`, to the status-line
command (https://code.claude.com/docs/en/statusline.md). Facts from a
spike on this host 2026-11-09 (Claude Code 2.1.296):

- `claude -p` never runs the status line. It cannot be used.
- An interactive `claude` started in a tmux window with
  `--settings <file>` whose `statusLine.command` writes stdin to a file
  does run it. The first status-line call has `rate_limits: null`.
  After one prompt is answered, the next call carries `rate_limits`.
  One tiny prompt ("Reply with the single word ok.") on
  `--model haiku` cost about $0.01 API-equivalent.
- The probe must run with every `SPORE_*` variable removed from its
  environment. With `SPORE_TASK_SLUG=coordinator` inherited, spore's
  hooks treated it as the coordinator, started `spore hooks
  watch-inbox`, and kept the process from exiting.
- A folder Claude Code has not seen shows a "do you trust this folder"
  prompt that blocks the session. The probe must run in one fixed folder
  that is trusted once.
- Running `claude` in a folder that is not a spore project root makes
  spore's Stop hook spawn a stray `spore/<dirname>/coordinator` tmux
  session. The 2026-11-09 spike did exactly that, even with `SPORE_*`
  removed. The probe must prevent it. Test first whether
  `disableAllHooks` in the probe's `--settings` still lets the status
  line run; if not, consider a separate Claude config directory that
  shares the login (careful: never symlink `~/.claude/plugins`). If
  neither works cleanly, ship the endpoint path first and the probe in
  a follow-up, and say so (builder's call, made by the coordinator).
- The probe must not use the operator's tmux session in a way that
  disturbs them: use a dedicated tmux server (`tmux -L <label>`), and
  tear down only that server by name.

## Where things live (decided)

- Code: extends `internal/budget` and `cmd/spore/budget_cmd.go`. No new
  top-level command.
- State: the existing budget state dir (`$AGENT_BUDGET_STATE_DIR`,
  default `~/.local/state/agent-budget`). Add `daily.json` (the current
  verdict) and `daily-history.jsonl` (one line per daily reading).
- Config: host-level, because the quota is per account, not per
  project: `~/.config/spore/budget.toml` with at least `margin` (percent
  points, default 10). Missing file means defaults.
- Schedule: a user systemd timer, `OnCalendar=*-*-* 18:00:00 UTC`,
  installed the same way as spore's other host timers.

## Interface (decided)

- `spore budget daily`: what the timer runs. Takes a fresh reading
  (primary, then fallback), computes the verdict, writes `daily.json`,
  appends one history line. Exit 0 when a verdict was written (room or
  not), non-zero when both sources failed. On total failure it still
  writes `daily.json` with verdict `unknown`.
- `spore budget room`: what the later feature calls. Reads `daily.json`
  only, never fetches. Exit 0 = `room`, exit 1 = `no-room`, exit 2 =
  `unknown` (no file, verdict `unknown`, or the reading is older than
  26 hours, meaning a daily run was missed). Once the saved
  `weekly_resets_at` is in the past, `room` answers `no-room` (exit 1)
  until the next daily reading, whatever the file says (operator
  decision 2026-11-09: the old week's headroom is gone, and right after
  a reset a fresh reading would say no room anyway). Prints one line:
  verdict, used, pace, headroom, reading time, source.

`daily.json` and each history line carry at least: `taken_at`,
`source` (`endpoint` or `probe`), `weekly_used`, `weekly_resets_at`,
`pace`, `headroom`, `margin`, `verdict`, `five_hour_used`,
`five_hour_resets_at`, plus any per-model weekly numbers the endpoint
returned.

## Constraints

- Go, inside this repo, following `internal/budget` conventions (state
  files 0600, atomic write via temp file plus rename).
- Spore is open-source bound: no host paths, hostnames or personal data
  in code, tests or docs.
- Every acceptance scenario below gets a test, written red first. Tests
  must not hit the real endpoint or start a real Claude session; inject
  the fetcher and the probe.

## Acceptance scenarios

1. Given the endpoint reports weekly 15% with reset 5 days away (pace
   ~28.6) and margin 10, when `spore budget daily` runs, then
   `daily.json` says `room`, headroom ~13.6, source `endpoint`, and
   `spore budget room` exits 0.
2. Given weekly 25% at the same point, then verdict `no-room` and
   `room` exits 1.
3. Given the endpoint fails and the probe returns weekly 15%, then the
   verdict is computed from the probe with source `probe`.
4. Given both fail, then `daily.json` says `unknown`, `daily` exits
   non-zero, and `room` exits 2.
5. Given `daily.json` is 27 hours old, then `room` exits 2 whatever it
   says.
5b. Given `daily.json` says `room` and its `weekly_resets_at` is one hour
   in the past, then `room` exits 1.
6. Given no config file, then margin 10 is used. Given `margin = 5`,
   then 5 is used.
7. Given the probe runs, then its process environment has no `SPORE_*`
   variable and it uses the dedicated tmux socket, never the default
   one.
8. End to end: with a fake endpoint server and a fake clock, run
   `daily` on three consecutive simulated days, then assert three
   history lines with rising pace and the right verdict each day.

## Left to the builder

- How the timer unit is installed (look at how existing spore user
  timers are created; `bootstrap/handover/systemd/` holds the reconcile
  timer template).
- How the probe waits for `rate_limits` (poll the capture file with a
  timeout; 90 seconds is a reasonable cap) and how it closes the session.
- Which folder the probe uses and how it is marked trusted once.
- Exact field names beyond the minimum list, and the one-line output
  format of `room`.
- Whether `daily` also refreshes the existing `state.json` as a side
  effect.
