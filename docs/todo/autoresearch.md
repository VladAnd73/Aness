**Status**: design, approved by nobody yet (2026-11-10). Not built.

# Autoresearch: an overnight improve-and-judge loop

## Intent

Andrej Karpathy's "autoresearch" setup (March 2026) runs a coding agent
all night with no human in the loop: one file the agent may edit, a
fixed time budget per experiment, a locked script that scores the
result as one number, keep the change if the number improves, reset
otherwise, log every try, repeat.

The operator wants the same loop inside spore. The first use is tuning
skills and rule files: today a skill change is tested by hand
(the knowledge-base sync skill went from 0/3 to 3/3 that way) and most
prompt edits are never measured at all. The loop turns "this edit
reads better" into "this edit scores better on a locked test set".

The operator was explicit that skills are only the first use. The core
must not know what a skill is: it edits one file, runs a judge that
prints a number, and applies an objective. Skills are the first
plug-in on top of that core.

## Goals

- A generic loop: one target file, one locked judge, one objective,
  keep or reset each try, everything logged.
- Deterministic code drives the loop. An agent is called only where
  code cannot do the job: proposing the next edit, running a scenario
  as the agent under test, and drafting the scenario set.
- The editing agent cannot cheat: it cannot see the scenarios, the
  checks, the holdout set, Linear, Notion, Slack, or the spore repo.
- Spending is bounded per night and per campaign, and nothing runs
  unless someone started it.
- A winner is checked against a sealed holdout set before anyone is
  asked to accept it.

## Non-goals

- Running every night there is quota room. Budget room is a gate,
  never a trigger.
- Editing more than one file per campaign.
- Targets other than skills in v1 (operator decision). Rule-pool
  fragments are the expected second plug-in, through the same core.
- Auto-applying winners. The live file changes only on the operator's
  `accept`.
- A scheduled timer. Nights start by hand (operator decision).

## Anti-goals (failure even if it "works")

- A winner that games the checks: passes the scenarios by matching
  what the checks look for, not by being a better skill.
- A winner that only learned the main scenario set and is no better
  (or worse) on unseen cases.
- A file that keeps growing because more text rarely lowers a pass
  rate. Every line of a skill is context every agent loads.
- A night that touches a real outside system (writes a Notion page,
  files a Linear issue, posts to Slack).
- A forgotten campaign quietly spending the week's quota.

## Concepts

- **Campaign**: one named run of the loop. Holds the target, the goal
  text, the judge, the objective, the limits, and its own git repo.
  Exactly one campaign may run a night at a time, host-wide (my call:
  they share one quota).
- **Target**: the one file the editor may change. Copied into the
  campaign repo at campaign start; the live file is never touched until
  `accept`.
- **Goal** (the `program.md` of the diagram): operator-written text
  saying what "better" means for this campaign, what to try, what is
  off limits. The editor reads it before every try.
- **Judge**: a locked command that takes a candidate file and prints
  one JSON result with a score. For skills it runs the main scenario
  set. Locked by a hash recorded at campaign start; a night refuses to
  start if the hash changed.
- **Holdout set**: a second scenario set written and approved with the
  main set, sealed, and run exactly once at campaign end.
- **Objective**: a named, pluggable rule that compares a try against
  the best so far. Two ship in v1 (see "Objectives").
- **Try**: one edit, one commit, one judge run, one keep-or-reset, one
  log line.
- **Night**: a sequence of tries started by hand, ending at a cap.

## Decisions taken by the operator

1. On/off is a per-campaign switch. A night runs only for a started
   campaign with a goal. `spore budget room` must also say `room`, but
   it never starts anything.
2. Deterministic code first; the agent only where needed.
3. The editing agent is isolated so it cannot see expected outputs or
   any source of hints (Linear and similar).
4. Grading: code checks first. Each scenario lists hard checks on what
   the agent under test did. An LLM grader is allowed only for a
   scenario code cannot check, and that scenario is marked as such.
5. Feedback to the editor after each try: the score only. Not the
   failed scenario names, not the check text.
6. Noise: each scenario runs 3 times per try. A try that beats the
   best so far triggers one full confirmation re-run; the change is
   kept only if it wins again.
7. Landing: a morning report (start and end score, diff, try log,
   holdout result). The operator runs an accept command to copy the
   winner into place, or to open a PR when the target is in the spore
   repo.
8. Outside systems: scenarios use fake services that return canned
   answers and record every call. Checks read the recorded calls.
9. Scenario authorship: a worker drafts the main and holdout sets from
   the skill, its known past failures and the goal. It proves the set
   discriminates (a deliberately broken copy of the target fails, the
   current target passes where expected). The operator approves. The
   set is then locked by hash for the whole campaign.
10. Spending caps, both kinds: a per-night try cap and wall-clock cap,
    plus a live weekly-quota ceiling read between tries. A campaign
    switches itself off after 3 nights in a row with no kept change, or
    after a maximum number of nights.
11. Length: ties go to the shorter file. A candidate more than 70%
    longer (in bytes) than the campaign's starting file is rejected
    whatever its score.
12. The objective is configurable per campaign, so one campaign can
    raise the score and a later one can hold the score while cutting
    length. The engine must be reusable for non-skill targets later.
13. Holdout: drafted up front with the main set, sealed, run once at
    the end, winner vs original.
14. The agent under test runs on the production model (today the one
    real workers use), so a score means "better for the agents that
    load this file".
15. Nights start by hand only: `spore research night <campaign>`. No
    timer.
16. v1 targets skills only. Other kinds come later as plug-ins.

## How a night runs

All steps are Go code in a new package (`internal/research`) unless
marked AGENT.

1. **Preflight**, refuse to start (non-zero exit, one-line reason) if:
   the campaign is not `on`; another night holds the host-wide lock;
   `spore budget room` does not exit 0; the judge hash or holdout hash
   differs from the one recorded at campaign start; the campaign repo
   has uncommitted changes.
2. **Baseline**: the first night of a campaign judges the starting
   file (with the confirmation re-run) and records it as best.
3. **Try loop**, until a cap hits:
   1. AGENT (editor): propose one edit to the target. Inputs: the
      target file, the goal, and the campaign's `results.tsv` (try
      number, score, kept/reset, length, and the editor's own one-line
      description of each earlier try). Nothing else.
   2. Commit the edit in the campaign repo.
   3. Length cap: if the file is more than 70% longer than the
      starting file, log `rejected-length`, reset, next try.
   4. Run the judge (each main scenario 3 times).
   5. Objective says "better than best"? Run the judge again in full.
      Kept only if the objective still says better on the re-run.
      The score of record is the mean of both runs (my call).
   6. Keep: the branch advances, best updates. Otherwise
      `git reset --hard` to the best commit.
   7. Append one line to `results.tsv`. A judge crash or timeout is
      logged as `crash` and reset; it never counts as a score.
   8. Check caps: try count, wall clock, quota ceiling.
4. **End of night**: write the night's section of the campaign report.
   Update the no-gain-night counter; switch the campaign `off` with a
   reason if the counter reaches 3 or the night count reaches the max.

## End of a campaign

When a campaign switches off (auto-stop or `spore research stop`) and
has a kept change:

1. Run the holdout set once on the original and once on the winner,
   with the same confirmation rule. Pass rule (my call): the winner
   must score at least as well as the original. Otherwise the report
   marks the winner `overfit` and recommends not accepting.
2. Write the final report: original vs winner on main and holdout
   scores, length change, the diff, the try log summary, nights used,
   quota used, and why it stopped.
3. Tell the coordinator (`spore task tell coordinator`) so the
   operator hears about it next morning.

`spore research accept <campaign>` then copies the winner over the
live file (keeping a backup next to it, my call) or, for a target
inside the spore repo, creates a branch and commit for a PR. It
refuses when the live file changed since the campaign started, and
when the holdout said `overfit` unless `--force` is given.

## Objectives

An objective is a Go interface: given the best result and a candidate
result, answer better / not better. v1 ships two:

- `maximize-score`: better when the score is higher; at an equal
  score, better when the file is shorter.
- `shrink-holding-score`: better when the file is shorter and the
  score is not lower than the campaign's starting score.

The 70% length cap applies under both. Adding an objective must not
require touching the loop.

## Judge contract (the generic seam)

The core runs the judge as a command:

```
<judge> --candidate <path> --set main|holdout --out <result.json>
```

`result.json` holds at least `score` (0..100), `runs`, `crashes`, and
cost and token figures. The core never reads scenarios itself. A
non-skill target later plugs in by supplying a different judge command
that honours this contract.

## The skill judge (first plug-in)

Built by extending `internal/evalharness` (`spore eval`), which already
spawns a real `claude -p` agent in a temp sandbox and grades with code
checks (files written, tool calls in the stream-json transcript, exact
output lines). Gaps it must close:

- **Scenarios as data, not Go.** Today scenarios are a hard-coded slice
  compiled into the binary. A campaign's scenario sets live in the
  campaign's own directory under spore's state dir (my call: never in
  the spore repo, so neither agent can find them in a checkout). Each
  scenario: a prompt, optional fixture files, which fake services it
  gets, and a list of checks.
- **Check kinds** (minimum): a file exists or matches; a recorded fake
  service call was or was not made with given fields; the transcript
  did or did not use a tool; the final reply contains or omits text.
  `llm-judge` is a separate, explicitly flagged kind with a locked
  rubric file.
- **Generic injection**: put the candidate file where the agent under
  test loads it (a skill under the sandbox's `.claude/skills/<name>/`,
  a rule in a composed CLAUDE.md), not just the two dream-brief slots
  supported today.
- **Repeats and a number**: N runs per scenario, score = percent of
  scenario runs that passed, JSON output, per-run timeout (the
  per-scenario budget, the diagram's "5 minutes"; default is the
  builder's call), cost captured from stream-json.
- **Model**: pass `--model` (production model per decision 14).
- **Isolation of the agent under test**: its own HOME and Claude
  config dir in the sandbox (today it shares the host's `~/.claude`),
  only the fake services via `--mcp-config` with
  `--strict-mcp-config`, no secrets in its environment. It must not be
  able to read the scenario store either: it would find its own checks.
- Fix while there: `execCommand` hard-codes `"claude"` and ignores
  `ClaudeSpawner.Binary`; `proposer.go` sets `XDG_STATE_HOME` with
  `os.Setenv` on the whole process.

### Fake services

A small MCP server inside spore (my call: one binary mode, e.g.
`spore research fake-mcp`) that serves the tools a scenario declares
(Notion, Linear and Slack shaped first), answers from the scenario's
canned responses, and appends every call to a calls log the checks
read. Unknown calls return an error and are logged too.

## Isolation of the editing agent

The editor is the one agent that sees the target, so it is the one
that could look for the answers. Two layers, both required:

- **Tools**: `claude -p` with `--tools` limited to Read, Edit and
  Write, no MCP servers (`--strict-mcp-config` with an empty config),
  `--setting-sources` excluding user settings (spore's user hooks would
  otherwise run, see the budget probe notes).
- **Filesystem**: run inside `bwrap` (bubblewrap, already on the host)
  with only the editor's scratch folder (target, goal, results.tsv)
  and the minimum Claude needs to log in bind-mounted. The scenario
  store, the holdout, the spore repo and the operator's home stay out
  of its view. Network: it needs the Anthropic API; whether other
  hosts are blocked is the builder's call, since with no shell and no
  web tools it has no way to reach them.

The agent under test gets the same treatment, with the fake services
added.

## Spending caps

Per campaign, in its config file, with these defaults (my calls,
operator may change per campaign):

- `max_tries_per_night = 20` (lowered after the builder check)
- `max_hours_per_night = 8`
- `quota_ceiling_points = 15`: stop the night when weekly usage has
  risen this many percentage points since the night started. Read
  between tries from the same OAuth usage endpoint `spore budget daily`
  uses; a failed read stops the night (fail closed).
- `max_nights = 7`, `stop_after_nights_without_gain = 3`.

## CLI (shape; names are the builder's call)

```
spore research new <campaign> --target <path> --goal <file> --objective <name>
spore research judge draft <campaign>     # mints a worker task to draft main + holdout sets
spore research judge approve <campaign>   # locks both by hash, campaign -> ready
spore research on|off <campaign>
spore research night <campaign>           # one night, foreground or in a tmux window
spore research status [<campaign>]
spore research report <campaign>
spore research stop <campaign>            # ends the campaign, runs the holdout
spore research accept <campaign> [--force]
```

## Storage

Under spore's state dir, per campaign (my call):
`research/<campaign>/` holding `campaign.toml` (config, status, hashes),
`repo/` (the campaign git repo with the target), `judge/main/`,
`judge/holdout/` (sealed), `results.tsv`, `report.md`, `nights/<n>/`
(per-try judge output, transcripts kept for one campaign then pruned).
Mode 0700 on the directory.

## Acceptance scenarios

1. Given a campaign that is `off`, when `night` runs, then it exits
   non-zero naming the reason and spawns no agent.
2. Given `spore budget room` exits 1 or 2, when `night` runs, then it
   refuses the same way.
3. Given a scenario file edited after approval, when `night` runs, then
   it refuses with a hash mismatch.
4. Given a fake editor that makes the file worse, when a try runs, then
   the campaign repo is reset to the best commit and `results.tsv` gets
   a `reset` line with the score.
5. Given a fake judge that scores a try higher once and lower on the
   confirmation re-run, when the try runs, then it is reset (lucky
   result rejected).
6. Given two candidates with equal score, when compared under
   `maximize-score`, then the shorter wins; given a candidate 71%
   longer than the start, then it is rejected without being judged.
7. Given an editor prompt that asks it to read a planted file in the
   scenario store, when it runs in its sandbox, then the read fails and
   the planted text never appears in its output (real `claude`, live
   test gated like today's `EVALHARNESS_LIVE`).
8. Given a scenario whose skill should create a Notion page, when the
   agent under test runs, then the fake service records the call, the
   check reads it, and no network call reaches a real Notion host.
9. Given a winner that scores lower than the original on the holdout,
   when the campaign stops, then the report says `overfit` and `accept`
   refuses without `--force`.
10. End to end: given a real skill with a known defect, an approved
    3-scenario set that catches it, and a scripted editor that fixes it
    on try 2, when `night` runs with `max_tries_per_night = 3`, then try
    2 is kept, tries 1 and 3 are reset, the night stops at the cap, the
    report shows the score rise, and `accept` writes the fixed file
    with a backup.
11. Given the quota ceiling is reached between tries, when the next try
    would start, then the night ends and the report says why.
12. Given 3 nights in a row with no kept change, when the third night
    ends, then the campaign is `off` with reason `no-gain`.

## Calls made after the builder check (mine, operator may overrule)

- **Login inside the sandboxes**: bind-mount `~/.claude/.credentials.json`
  read-only (the same account `spore budget` measures), nothing else
  from the operator's home. "No secrets" means no credentials for any
  outside service (GitHub, Notion, Linear, Slack); the Anthropic login
  is the one exception.
- **Agent under test, tools**: the normal Claude Code tools including
  Bash (real skills need it), plus the fake services registered under
  the exact tool names the skill text uses (for example
  `mcp__claude_ai_Notion__*`), otherwise a score measures a name gap,
  not the skill. The account's claude.ai connectors must be off in the
  sandbox. Acceptance scenario 8 must prove with a live run that no
  request reaches a real outside service; how (no credentials, network
  filtering, or both) is the builder's call.
- **Judge pinning**: `judge approve` copies the spore binary into the
  campaign directory and records its hash with the scenario hashes. The
  judge always runs from that copy, so a host upgrade mid-campaign
  neither changes the grading nor blocks a night.
- **Parallel runs**: scenario runs inside one judge run go in parallel,
  default limit 3 (campaign setting). Per-run timeout default 5 minutes.
  Defaults change to `max_tries_per_night = 20`; a night that reaches
  the hour cap first is normal.
- **Skill folders**: the target is one file, but the rest of its folder
  is frozen into the campaign repo at campaign start. The editor may
  read those files and not change them. The agent under test gets the
  frozen folder with the candidate file in place. `accept` refuses when
  anything in the live folder changed since campaign start.
- **Scenario drafting**: `judge draft` mints an ordinary worker task in
  the spore project. The worker writes drafts into a git-ignored folder
  of its worktree. `judge approve` imports them into the campaign's
  store, then locks them. Neither sandboxed agent can see worktrees.

## Left to the builder

- Exact CLI names and flags, file formats, the per-scenario timeout.
- How the editor's one-line try description is captured.
- Whether the editor is called with a fresh session per try (expected)
  or resumed.
- Network policy for the two sandboxes beyond "fake services only and
  no hints reachable".
- Pruning of transcripts.
