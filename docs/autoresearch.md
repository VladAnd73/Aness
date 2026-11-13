# Autoresearch: improving a skill overnight

`spore research` runs an overnight loop on one skill file. Each try, an
editing agent proposes one edit; a locked judge scores the edit on a
set of scenarios; the loop keeps the edit only when it is confirmed
better and resets it otherwise. When the campaign ends, a sealed
holdout set checks the winner, and the winner lands only when you run
`accept`.

The design and its decisions live in `docs/todo/autoresearch.md`.

## What a campaign is

A campaign is one skill, one goal and one judge, plus everything the
nights produce. v1 targets skills only: the target must be a file named
`SKILL.md`. Its whole folder is copied into the campaign; the editor may
change `SKILL.md` only, the other files are reference material.

Campaign states: `draft` (no judge yet), `ready` (judge approved),
`on` (nights may run), `off` (switched off by you or by a cap, can go
back on), `stopped` (ended, holdout run), `accepted` (winner landed).

## Walk-through

```
spore research new demo --target ~/skills/demo/SKILL.md --goal goal.md
spore research judge draft demo        # from the spore project root
# a worker drafts research-drafts/demo/{main,holdout}/ in its worktree
spore research judge approve demo --from <worktree>/research-drafts/demo
spore research set demo model haiku    # optional; see "Settings"
spore research on demo
spore research night demo              # in a tmux window; one per night
spore research status
spore research report demo
spore research stop demo               # runs the holdout, tells the coordinator
spore research accept demo
```

- `new` takes `--objective` (`maximize-score`, the default, or
  `shrink-holding-score`) and `--model`.
- `judge draft` mints a draft task in `./tasks` and prints its slug. It
  does not start it: read it, then `spore task start <slug>`. The brief
  asks for scenarios grounded in the skill's past failures and for a
  discrimination proof: a deliberately broken copy of the skill must
  fail where the current skill passes. The drafts stay in the
  git-ignored `research-drafts/` folder and are never committed.
- `judge approve` copies both sets into the campaign, pins a copy of
  the running `spore` binary as the judge, and records the hash of all
  three. Any later change to them makes `night` refuse.
- `night` runs in the foreground. Start it in a tmux window
  (`tmux new-window -d -n research "spore research night demo"`). It
  prints one line per try:

  ```
  try 3  kept             score 83.3  bytes 1412  elapsed 14m02s
  ```

  Ctrl-C, SIGTERM and a closed tmux window (SIGHUP) all end the night
  cleanly: the judge is killed, the campaign repo goes back to the
  best commit, the lock is released, and no half line is written.
  Exit 0 means the night ran; exit 1 means it was refused, with the
  reason on stderr.
- `stop` ends the campaign. With a kept change it runs the holdout set
  twice on the original and twice on the winner. The winner passes when
  it scores at least as well as the original; otherwise the report
  says `overfit` and recommends not accepting. Then it tells the
  coordinator through `spore task tell coordinator`, run from
  `$SPORE_PROJECT_ROOT` (or the current directory when unset). A failed
  notice is only a warning in the report. A failed holdout leaves the
  campaign as it was, so `stop` can be retried.
- `accept` needs a stopped campaign with a kept change, refuses a
  second time, refuses when any file in the live skill folder changed
  since `new`, and refuses an `overfit` verdict unless `--force`.
  Outside the spore repo it overwrites `SKILL.md` and keeps the old one
  beside it as `SKILL.md.research-<campaign>.bak`. Inside the spore
  repo it never touches your checkout: it commits the winner on a new
  branch `research/<campaign>` through a temporary worktree. To turn it
  into a PR:

  ```
  git push -u origin research/demo
  gh pr create --head research/demo
  ```

## Settings

`spore research set <c> <key> <value>` changes one setting while no
night runs (refused on a stopped or accepted campaign). Values are
plain: `set demo model haiku`, `set demo run_timeout 3m`.

| Key | Default | Meaning |
|-----|---------|---------|
| `max_tries_per_night` | 20 | tries before the night stops |
| `max_hours_per_night` | 8 | wall-clock hours before the night stops |
| `quota_ceiling_points` | 15 | stop when weekly usage rose this many points since the night began |
| `max_nights` | 7 | nights before the campaign switches off |
| `stop_after_nights_without_gain` | 3 | switch off after this many nights in a row with no kept change |
| `repeats` | 3 | runs per scenario per judging |
| `parallel` | 3 | scenario runs at once |
| `run_timeout` | 5m | limit for one scenario run |
| `model` | (none) | `--model` for the editor and the agent under test; empty uses the CLI default |
| `agent_binary` | `claude` | the claude binary for both agents |
| `isolation` | `bwrap` | `none` runs both agents unsandboxed and needs `SPORE_RESEARCH_ALLOW_UNSANDBOXED=1` |

A change to `model`, `agent_binary`, `isolation`, `repeats`,
`parallel` or `run_timeout` also rebuilds the judge command, as long as
it is still the default `<pinned spore> eval judge ...` one.

## Before a night starts

`night` refuses, naming the reason, when: the campaign is not `on`;
`isolation = none` without the opt-in, or bubblewrap is not usable;
another night holds the host-wide lock (`$XDG_RUNTIME_DIR/spore-research.lock`);
`spore budget room` does not say room; a judge hash differs; the
campaign repo has uncommitted changes; the goal file is missing or
empty; the weekly usage cannot be read.

Between tries the night stops for, in this order: interrupted, a failed
usage read (`quota-read-failed`, it never continues blind), the quota
ceiling, the try cap, the hour cap. The first night of a campaign first
judges the starting file twice and records the mean as the baseline
(try 0).

### Recovering from a kill -9

A night killed with SIGKILL (or a host crash) cannot clean up, so the
campaign repo may hold a half-made try, and the next `night` refuses
with "campaign repo has uncommitted changes". Run:

```
spore research reset demo
```

It puts the repo back on the best commit recorded in `campaign.toml`
and is refused while any night holds the lock.

## Storage

Everything lives in `$XDG_STATE_HOME/spore/research/<campaign>/`
(fallback `~/.local/state/spore/research/<campaign>/`), directories
0700 and files 0600, never inside the spore repo. `SPORE_RESEARCH_DIR`
moves the store (for a trial run) without moving the coordinator inbox,
which a throwaway `XDG_STATE_HOME` would also move:

```
campaign.toml        settings and progress (flat key = value)
goal.md              copy of the goal
repo/                git repo of the skill folder; "start" is commit 1
judge/main/          the approved main set (hashed)
judge/holdout/       the sealed holdout set (hashed)
bin/spore            the pinned judge binary (hashed)
results.tsv          one row per try, all nights
report.md            one section per night, then the final section
nights/<n>/          per night: baseline-*.json, try-<k>/ (scratch,
                     editor.jsonl, judge-*.json, crash.txt)
holdout/             the holdout runs from stop
```

## What each sandbox sees

Both agents run under bubblewrap with a private HOME, a read-only copy
of fresh login credentials, and no user settings or hooks.

- The editor sees only its scratch folder: `SKILL.md` (writable),
  `goal.md`, a cut-down `results.tsv` (try, score, status, bytes,
  description), and the rest of the skill folder read-only under
  `frozen/`. Its tools are Read, Edit and Write; it has no MCP servers,
  no shell and no web tools. It never sees scenario names, prompts or
  checks.
- The agent under test sees its own working directory with the
  candidate installed as `.claude/skills/<name>/SKILL.md`, the
  scenario's fixture files, and only the fake MCP services the
  scenario declares. It cannot see the scenario store, the holdout,
  the spore repo or your home.

## Reading results.tsv and report.md

`results.tsv` columns: `try` (0 is the baseline; numbering restarts
each night), `at`, `score` (0 to 100, percent of scenario runs that
passed; the mean of both judgings for a kept try; `-` when not judged),
`status`, `bytes`, `commit`, `description` (the editor's one line).

Statuses: `baseline`; `kept` (better on two judgings in a row);
`reset` (not better on the first or the confirming judging);
`rejected-length` (more than 70% longer than the starting file, not
judged); `crash` (the editor failed, broke the target, touched a frozen
file, or the judge failed; the repo is reset and the night goes on).

`report.md` gets a section per night (stop reason, tries, kept, score
change, quota points used, judge cost so far) and, at `stop`, a final
section with start and best score, holdout verdict, length change, try
counts, quota and cost, and the diff of the winner.

## Fake services

`spore research fake-mcp --services tools.json --calls-log calls.jsonl`
serves canned MCP tools on loopback, prints `<server> <url>` lines, and
logs every call. The judge starts its own fake per scenario from the
scenario's `services`; the command is for writing and trying scenarios
by hand.

## Scenarios

A set is a folder of scenario folders, each with a `scenario.json` and
an optional `files/` folder copied into the run first:

```json
{
  "name": "s1",
  "prompt": "Record the run status. Use the demo skill.",
  "services": [{"server": "claude_ai_Notion", "name": "notion-search", "response": {"results": []}}],
  "checks": [{"kind": "file-matches", "path": "out.txt", "pattern": "^status: done$"}]
}
```

Check kinds: `file-exists`, `file-matches`, `service-call` (tool plus
`fields`), `tool-used`, `reply-contains`, each negatable with
`"negate": true`, and `llm-judge` with a `rubric`, allowed only when
the scenario sets `"llm_judge": true`. Try a set by hand with
`spore eval judge --scenarios-root DIR --skill NAME --candidate SKILL.md --set main --out r.json`.

## Extending

- **An objective**: implement `research.Objective` (`Name`, and
  `Better(start, best, candidate Measured) bool`) and call
  `research.RegisterObjective` from an `init`. The loop does not change;
  the 70% length cap applies to every objective.
- **A non-skill judge**: any command that takes
  `--candidate PATH --set main|holdout --out FILE` and writes JSON with
  at least `score` (0 to 100) and `cost_usd`, exiting 0 whatever the
  score. Pass it as the judge argv to `research.Store.ApproveJudge`; the
  CLI only builds the default skill judge today, and `new` still
  accepts only `SKILL.md` targets.
