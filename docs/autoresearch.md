# Autoresearch: improving a skill or a role overnight

`spore research` runs an overnight loop on one skill file or one loop
role body. Each try, an
editing agent proposes one edit; a locked judge scores the edit on a
set of scenarios; the loop keeps the edit only when it is confirmed
better and resets it otherwise. When the campaign ends, a sealed
holdout set checks the winner, and the winner lands only when you run
`accept`.

The design and its decisions live in `docs/todo/autoresearch.md`, and
for role targets in `docs/todo/autoresearch-roles.md`.

## What a campaign is

A campaign is one target, one goal and one judge, plus everything the
nights produce. A skill target (the default, `--kind skill`) must be a
file named `SKILL.md`. Its whole folder is copied into the campaign; the
editor may change `SKILL.md` only, the other files are reference
material. A role target (`--kind role`) is a loop role body; see "Role
targets" below.

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
  twice on the original and twice on the winner. The winner passes only
  when it scores higher than the original: a tie is `overfit`, and the
  report recommends not accepting. When the original already scores
  100 on the holdout, the verdict is `no-headroom`: the set cannot show
  a gain, so make it harder. Then it tells the
  coordinator through `spore task tell coordinator`, run from
  `$SPORE_PROJECT_ROOT` (or the current directory when unset). A failed
  notice is only a warning in the report. A failed holdout leaves the
  campaign as it was, so `stop` can be retried.
- `accept` needs a stopped campaign with a kept change, refuses a
  second time, refuses when any file in the live skill folder changed
  since `new`, and refuses an `overfit` or `no-headroom` verdict unless
  `--force`. For a skill outside the spore repo (a dotfiles repo
  included) it overwrites `SKILL.md` and keeps the old one beside it as
  `SKILL.md.research-<campaign>.bak`. For a skill inside the spore repo,
  or a role body inside any git checkout, it never touches your
  checkout: it commits the winner on a new branch `research/<campaign>`
  of that repo through a temporary worktree. The live skill folder (for
  a role, the role body) must match `HEAD` (no uncommitted or untracked
  files), since the branch is cut from `HEAD`. For the same reason `new`
  refuses a skill folder in the spore repo that holds a gitignored file
  (and a gitignored role body): the judge would score with it, but the
  branch never holds it. Remove it, or commit it with `git add -f`. A branch
  `research/<campaign>` that already holds this winner (a run that died
  after committing) is reused; any other branch of that name is refused.
  To turn it into a PR:

  ```
  git push -u origin research/demo
  gh pr create --head research/demo
  ```

## Settings

`spore research set <c> <key> <value>` changes one setting while no
night or stop of that campaign runs (refused on a stopped or accepted campaign). Values are
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
it is still the default `<pinned spore> eval judge ...` one. These six
change what the judge measures, so they are refused once the baseline
is scored: the best score would be stale.

## Before a night starts

`night` refuses, naming the reason, when: the campaign is not `on`;
`isolation = none` without the opt-in, or bubblewrap is not usable;
another night holds the host-wide lock (`$XDG_RUNTIME_DIR/spore-research.lock`);
`spore budget room` does not say room; a judge hash differs; the
campaign repo has uncommitted changes; the goal file is missing or
empty; the weekly usage cannot be read; a role campaign's step in
`campaign.toml` differs from `context/step.json`; the baseline already
scores 100 under `maximize-score` (`no-headroom`).

When the first night's baseline scores 100 on the main set under
`maximize-score`, the night ends right after it with `no-headroom`,
before any edit: it switches the campaign `off` and exits 1. Nothing can
show a gain there. Draft harder scenarios and run `judge approve` again:
it is allowed on any on or off campaign whose baseline is 100 with
nothing kept, sets it back to `ready`, restarts the night and no-gain
counts (quota and cost stay on the books), and the next night scores a
fresh baseline. A kept try that reaches 100 under `maximize-score` ends
the night with `max-score` and ends the campaign like a limit does: the
holdout runs and the coordinator is told. If that night was interrupted
before the holdout, `on` is refused; run `spore research stop`.
`shrink-holding-score` is not capped: it can still shorten a file at
100.

Between tries the night stops for, in this order: interrupted, a failed
usage read (`quota-read-failed`, it never continues blind), the quota
ceiling, the campaign switched `off` (`switched-off`; `off` during a
night takes effect after the current try and stays off), the try cap,
the hour cap. A judge that fails outright, before scoring anything (a
missing grader, a sandbox that will not start, credentials too short),
logs a `crash` try; two such tries in a row end the night with
`judge-failed` (a try that fails before the judge breaks the run). The
baseline retries one such failure; two in a row end its night with
`baseline-crash`. A judge whose runs merely crashed is a `crash` try. Every night counts toward `max_nights`,
including one whose baseline crashed (`baseline-crash`; the next night
retries the baseline). A night stopped by `interrupted`,
`quota-read-failed`, `switched-off`, `baseline-crash` or `judge-failed`,
or switched `off` during the night whatever stopped it, does not add to
the no-gain count when nothing was kept. A manual `off` is a pause and
does not run the holdout, unless that night reached `max_nights` or
the no-gain limit: then the campaign ends as usual. An interrupted
night that reaches a limit with a kept change cannot run the holdout:
it leaves the campaign `off`, and the report and the coordinator notice
say to run `spore research stop <campaign>`, which runs the holdout and
ends it. Until then `on` is refused with the same advice, as it is after
a holdout that failed and for any `off` campaign with a kept change
whose `max_nights` or `stop_after_nights_without_gain` is reached;
raising that setting with `set` lets it go back `on`. Each holdout side
(original, winner) is kept in `holdout/result.json` once paid for, so a
`stop` retried after a later failure does not run and pay for it again.

Nights spend the one shared weekly quota, so only one night or `stop`
runs at a time on the host (`$XDG_RUNTIME_DIR/spore-research.lock`); a
night or `stop` that its campaign's status refuses never takes it.
Settings are per campaign: `on`, `set`, `reset`, `judge approve` and
`accept` are refused only while a night, `stop` or other setting change
of that same campaign runs (its `night.lock`), never because another
campaign's night runs. A refusal names what holds the lock, such as
"the stop of demo".
The first night of a campaign first judges the starting file twice and
records the mean as the baseline (try 0).

### Recovering from a kill -9

A night killed with SIGKILL (or a host crash) cannot clean up, so the
campaign repo may hold a half-made try, and the next `night` refuses
with "campaign repo has uncommitted changes". Run:

```
spore research reset demo
```

It puts the repo back on the best commit recorded in `campaign.toml`
and is refused while a night or stop of that campaign runs.

## Storage

Everything lives in `$XDG_STATE_HOME/spore/research/<campaign>/`
(fallback `~/.local/state/spore/research/<campaign>/`), directories
0700 and files 0600, never inside the spore repo. `SPORE_RESEARCH_DIR`
moves the store (for a trial run) without moving the coordinator inbox,
which a throwaway `XDG_STATE_HOME` would also move:

```
campaign.toml        settings and progress (flat key = value)
campaign.lock        held for each read-modify-write of campaign.toml
night.lock           held by this campaign's night or stop
goal.md              copy of the goal
repo/                git repo of the skill folder; "start" is commit 1
judge/main/          the approved main set (hashed)
judge/holdout/       the sealed holdout set (hashed)
bin/spore            the pinned judge binary (hashed)
results.tsv          one row per try, all nights
report.md            one section per night, then the final section
nights/<n>/          per night: baseline-*.json, try-<k>/ (scratch,
                     editor.jsonl, judge-*.json, crash.txt)
holdout/             the holdout runs from stop; result.json once done
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
A judging where every run crashed (an expired login, say) is a judge
failure, never a score of 0.

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
the scenario sets `"llm_judge": true`. The grader model gets no tools
and, unless `isolation = none`, runs in the same sandbox as the agent
under test, since it reads that agent's untrusted reply. Try a set by hand with
`spore eval judge --scenarios-root DIR --skill NAME --candidate SKILL.md --set main --out r.json`.

## Role targets

A role campaign improves the body of one loop step's role, such as a
project's `qa-analyst` (step `intake` of its `qa` loop):

```
spore research new qa1 --kind role --step intake \
  --target ~/myproject/bootstrap/roles/qa-analyst.md --goal goal.md
spore research judge draft qa1          # mints the corpus-builder brief
```

- `new` finds the loop file in the nearest `bootstrap/loops/` above the
  target holding an agent step named `--step` (`--loop NAME` when more
  than one does) whose role is the target's file name. It pins the
  step's output, shape and skills in `campaign.toml` and copies the loop
  file, `step.json` and the skills (from the project's, then your
  `.claude/skills/`) into the campaign repo under `context/`. A skill it
  cannot find, such as a plugin's, is named in a warning and the judge
  runs without it. A later edit to the loop file does not change the
  campaign.
- The editor sees the role body and, read-only under `frozen/context/`,
  the loop file, the step and the skills.
- `judge draft` mints a corpus-builder brief: mine the project's
  tracker for bugs filed by its QA workers and bugs reported by
  customers and developers, keep those with a linked feature ticket,
  repro steps and an identifiable fix, write one scenario per
  bug at its pre-fix commit, split main and holdout (holdout at least
  30% and 5 bugs, one feature on one side), and prove a weakened role
  body scores lower. Corpus content is never committed.
- `accept` commits the winner on `research/<campaign>` in the role's own
  repo, ready for a PR there.

### What a role run sees

Each run lays out a loop step's world in the sandbox: the scenario's
`snapshot/` as the working directory (no `.git`), its `spec.md` at
`$SPORE_TASK_DIR/spec.md` (`<run>/.spore/replay/`), the frozen skills
under `.claude/skills/`, and env `SPORE_TASK_DIR`, `SPORE_ROLE`,
`SPORE_LOOP`, `SPORE_STEP`, `SPORE_STEP_OUTPUT` and `SPORE_STEP_SKILLS`.
The prompt is the loop's step note for round 1 (output path and shape),
a blank line, then the candidate body, exactly as a loop pane gets it.
The scenario's `grader/` is never copied in; only the grader reads it.

### Role scenarios

```
<set>/<bug>/scenario.json   {"kind": "role", "llm_judge": true, "checks": [...]}
<set>/<bug>/spec.md         the feature's task spec, written before the bug was known
<set>/<bug>/snapshot/       the code at the fix commit's parent, sparse, no .git
<set>/<bug>/grader/         repro.md: repro steps, expected vs actual, bug id, pre-fix commit
```

A role scenario has no `prompt`. Its checks:

- `output-valid`: the round-1 step output exists and passes the loop's
  own step-output parser for the step's shape.
- `bug-caught` (needs `llm_judge`; `repro` names a file under
  `grader/`): the grader sees the repro steps and every file the agent
  wrote or changed, and must end with `QUOTE: <line>` and
  `VERDICT: CAUGHT`. A catch without a quote, or with a quote not found
  in the output, counts as missed. The quotes land in the judge result
  for audit.
- `fluff` (`path` relative to `$SPORE_TASK_DIR`, `pattern`,
  `reasonable`): fails when the file holds more than 1.5 times
  `reasonable` lines matching `pattern`, or none.
- `code-quality` (optional `rules`): fails, naming each rule, when a
  source file the agent added or changed breaks `no-added-comments`,
  `no-visibility-only-asserts` (a test whose every assertion only
  checks that an element shows), `no-duplicate-helpers` (a function or
  method whose name the snapshot already defines) or `no-api-end-state`
  (an assertion on an API response).
- `llm-judge` grades the same output files with a rubric, for example
  the 0 to 3 readability rubric for qa-author.

A fake service may be keyed: `{"server": "claude_ai_Linear", "name":
"get_issue", "key": "id", "responses": {"FAKE-1": {...}}}` answers only
the listed ids and returns a not-found error for any other, so the
agent cannot read the bug ticket.

## Extending

- **An objective**: implement `research.Objective` (`Name`, and
  `Better(start, best, candidate Measured) bool`) and call
  `research.RegisterObjective` from an `init`. The loop does not change;
  the 70% length cap applies to every objective.
- **A non-skill judge**: any command that takes
  `--candidate PATH --set main|holdout --out FILE` and writes JSON with
  at least `score` (0 to 100) and `cost_usd`, exiting 0 whatever the
  score. Pass it as the judge argv to `research.Store.ApproveJudge`; the
  CLI builds only the default skill and role judges.
