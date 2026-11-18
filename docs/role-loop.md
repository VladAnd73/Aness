# Role-based fleet: operator guide

The role loop runs one task through an engineer pane and two reviewer
panes. Roles never see each other; they talk through files under
`.spore/<slug>/`. The driver derives all state from that tree, so
every command here is safe to re-run.

## Minting a task

Create `tasks/<slug>.md` with the role opt-in:

    spore task new "<title>" --role

The frontmatter is the standard task set (`status`, `slug`, `title`,
`created`, `project`) plus `loop: role`; the body below the
frontmatter becomes the spec. When editing a task file by hand, add
`loop: role` to the frontmatter yourself.

Then flip the frontmatter to `status: active` by editing the file,
not via `spore task start` (start spawns a homogeneous worker
session, and refuses on a `loop: role` task). The key makes the
opt-in atomic with the status flip: the reconciler pass that reacts
to the file write already sees `loop: role`, never mints a
homogeneous worker for the slug, and drives the role loop instead.
No pre-seeding is needed; the first drive tick creates
`.spore/<slug>/`.

`loop: role` is the only opt-in. A `.spore/<slug>/` dir on disk does
not mark the slug role-looped: coordinators stage briefs and inputs
there for generic tasks too, and those tasks keep their single
homogeneous worker. A task seeded by hand with only the dir needs
`loop: role` added to enter the role loop.

`role-drive` is also the manual driver: each run advances the loop by
one idempotent tick, and refuses a task whose frontmatter lacks
`loop: role`. The reconciler calls the same drive for
role-looped slugs on every pass, so an active task keeps moving
without manual ticks.

The first tick:

- caches the task body (frontmatter stripped) at
  `.spore/<slug>/spec.md`; the cache is immutable for the run
- creates the `wt/<slug>` branch and worktree at `.worktrees/<slug>/`
- spawns the engineer pane, tmux session
  `spore-role/<project>/<slug>/engineer`

## The phase machine

Phases derive from artifact counts on disk; writing a file causes the
transition. Verdict files hold `approve` or `request_changes`.

| Phase | Meaning | Entered by writing |
|---|---|---|
| `engineer-initial` | no engineer response yet | (start state) |
| `review-A` | A owns the next verdict | `responses/engineer-round-N.json` |
| `engineer-revise-A` | A requested changes | `reviews/A/round-N.json` (request_changes) |
| `review-B` | A approved; B owns the verdict | `reviews/A/round-N.json` (approve) |
| `engineer-revise-B` | B requested changes | `reviews/B/round-N.json` (request_changes) |
| `done` | B approved; branch ready | `reviews/B/round-N.json` (approve) |
| `escalated` | cap hit; operator decides | request_changes at round 3 |

A revise phase flips back to the review phase once the engineer
writes the response to the latest verdict. Each reviewer phase is
capped at 3 rounds (`MaxReviewerRounds`): a `request_changes` verdict
at round 3 escalates instead of looping. On escalation the driver
leaves all panes alive; the operator can clarify the spec,
force-approve, kill the task, or hand it back for another loop.

Per-phase side effects of a drive tick:

- engineer phases: keep the engineer pane alive (see Pane liveness);
  on revise phases, nudge it (tmux send-keys) with the latest verdict
  path
- `review-A`: keep engineer and reviewer A panes alive
- `review-B`: reap reviewer A's pane, keep reviewer B alive; the
  engineer pane stays alive across the handover
- `done`: write `summary.md`, clear `escalated-*` markers, write the
  `ready` marker, reap all three panes
- `escalated`: write the `escalated-*` marker, touch nothing else
- every phase: bounce invalid role files (see below)
- non-terminal phases: check the active role for a stall (see Pane
  liveness)

## Pane liveness

Phase completion comes only from files; a pane exiting never advances
or stalls the loop by itself. Each tick probes every pane the phase
expects (tmux `pane_dead`, `pane_current_command`, `session_created`):

- missing session: spawn it, silently. This covers a pane missing
  after a phase transition and a token-monitor wrap (which kills its
  own session on purpose); neither counts toward the respawn cap.
- agent gone (pane dead, or a shell in the foreground once the
  session is 30s old, e.g. a hand-started role whose agent exited):
  kill the session and spawn a fresh one.

Each agent-gone respawn claims `state/respawn-<role>-r<round>-<n>`
and tells the coordinator (`<slug>: respawned <role> (<reason>; round
R, respawn K/3)`). When the agent dies again after 3 respawns of one
role in one round, the driver stops, leaves the dead pane for
inspection, and tells the coordinator once (`<slug>: escalated:
<role> agent died again ...`, marker
`state/respawn-capped-<role>-r<round>`). The cap re-arms when the
role's round advances; delete the role's `respawn-<role>-r<round>-*`
markers and the capped marker to re-arm it by hand.

Stall: the role that owns the next file write is stalled when its
expected file is absent and neither the role files under
`.spore/<slug>/` (outside `state/`) nor the pane transcript (the text
above the input prompt; status line and input box excluded) changed
for `[fleet] role_stall_minutes` (default 20, 0 disables). The
coordinator is told once per quiet spell (`<slug>: stalled: ...`);
any activity re-arms it. Stalls never kill the pane: the agent may be
waiting on a dialog only the operator can answer. The activity clock
lives in `state/activity-<role>-r<round>`.

## Roles

All panes run in the task worktree with `SPORE_TASK_DIR` pointing at
the absolute `.spore/<slug>/` tree, plus `SPORE_TASK_SLUG`,
`SPORE_PROJECT_ROOT`, `WT_PROJECT`, and `SPORE_ROLE` in the
environment. Role bodies are bundled in the spore binary; a project
file at `bootstrap/roles/<role>.md` overrides the bundled body of the
same name. A bundled body is written to
`$XDG_STATE_HOME/spore/<project>/roles/<sha>/<role>.md`, keyed by
content so two spore releases spawning for one project never swap each
other's body. An override that exists but is broken (dangling symlink
on the file or the `bootstrap/roles` dir, unreadable, not a regular
file) fails the spawn instead of falling back; a `bootstrap` or
`bootstrap/roles` that is not a directory (such as a `./bootstrap`
script) means no override. A project with a `bootstrap/roles/` dir
that lacks one body gets a stderr warning, once per process, when the
bundled body fills the gap. See "Reviewer slots" for the code-review
body.
When `[fleet] account_tier` is set in spore.toml, panes also get
`SPORE_ACCOUNT_TIER`. The token monitor does not read it: role panes
are metered at the worker wrap cap (see the end of this file).

### Reviewer slots

Each reviewer slot (A, B) picks how it produces its verdict, in
spore.toml or the host overlay (`$XDG_STATE_HOME/spore/<project>/workers.toml`,
which wins key by key):

```toml
[fleet.reviewers.A]
mode   = "code-review"  # "fleet" | "code-review" (A defaults to code-review, B to fleet)
agent  = "claude"       # optional; default is the task / [fleet.workers] agent
effort = "medium"       # code-review only: low|medium|high|xhigh|max (default medium)

[fleet.reviewers.B]
mode = "fleet"
```

- `fleet` runs the `reviewer.md` body: spec conformance, runs tests
  and lints. B defaults to it.
- `code-review` runs the `reviewer-code-review.md` body, which
  invokes Claude Code's `code-review` skill at `effort` on the branch
  diff (on the draft PR with `--comment` when `github.json` holds a
  PR number). Findings map to the verdict: any non-cosmetic finding
  is `request_changes`, none or cosmetic-only is `approve`, one
  `path:line: text` comment per finding. The verdict file schema and
  the `Review #1` / `Review #2` mirror are unchanged. The run has no
  spec context, so the engineer pushes back with a one-line reason
  on findings the spec contradicts, and later rounds do not re-raise
  a finding with a sound pushback.
- `code-review` is claude-only. A defaults to it (effort `medium`)
  when no `mode` is set, but falls back to `fleet` when its agent
  (slot `agent`, else the task `agent:`, else the `[fleet.workers]`
  default) resolves to non-claude; an `effort` it set is then
  ignored with a stderr warning, as on any fleet slot. A loop step
  on slot A whose role is not `reviewer` falls back to `fleet` the
  same way, since only the reviewer has a code-review body. The fallback
  is re-evaluated at every spawn, like the agent, so changing the
  task agent or `[fleet.workers]` default mid-task can switch A
  between the two protocols across rounds; pin `mode` on A to rule
  that out. An explicit `mode = "code-review"` with a non-claude
  `agent` fails config load; without `agent` it runs claude even
  when the task pins codex.
- `agent` on a slot overrides only that slot. It is resolved at each
  spawn from the current config, like the task agent, and never from
  the worker ratio: a live pane keeps its agent, but a pane spawned
  after a config edit picks up the new one.
- Panes get `SPORE_REVIEWER_MODE`, plus `SPORE_REVIEW_EFFORT` in
  code-review mode.

The default pairing is A = code-review, B = fleet: bug hunt first,
spec and verification second. spore's own spore.toml pins it
explicitly.

### Pre-start hook

`[fleet] pre_start = "<shell command>"` runs a project command inside
each role pane and each worker session, before the agent starts (not
the coordinator). Typical use is bringing services up and loading a
test schema, e.g. marketer:

    [fleet]
    pre_start = "bin/with-services up && <schema load>"

The command runs in the session cwd (the worktree) with the session's
`SPORE_*` env, via `sh -c`, under `flock .spore/<slug>/state/pre-start.lock`.
The lock is per slug and shared by the worker, engineer, and both
reviewers, so make the command idempotent: later sessions wait for
the first, then should return quickly. The pane shell runs it, not
the reconciler, so spawning never blocks on it. The hook runs under
`timeout`, default 900s, set by `[fleet] pre_start_timeout_seconds`
(0 runs it unbounded), so a hung bring-up cannot hold the slug lock
forever.

While a session waits on the lock or runs the hook, the pane shows
the launch shell, which liveness would otherwise read as a dead
agent once spawnGrace passes. The prelude therefore holds
`state/pre-start-<role>.running` (`engineer`, `reviewer-A`,
`reviewer-B`, or `worker`) from just before the lock until just
before exec. Liveness leaves a shell-showing pane alone while that
marker exists and is no older than the session; a marker left by a
pane killed mid-hook is older than the respawned session and is
ignored.

A non-zero exit (including a missing `flock`, or 124 on timeout) never stops the agent.
The agent's first prompt gets one leading line: `spore: pre-start
hook failed (exit N): <command>. Services may be down.` Role panes
put it above the role body; a role pane with no role body, and a
worker (which starts with no prompt), get that line alone as the
prompt. Unset or empty leaves the spawn argv byte-identical to
before.

With `[worker] isolate_network = true` the whole launch script, hook
then agent, runs inside one pasta network namespace, so services the
hook starts on loopback are the ones the agent reaches (pasta runs
with port forwarding off, so a hook run on the host would be out of
the agent's reach). Only the agent gets `IS_SANDBOX=1`.

Engineer (`spore-role/<project>/<slug>/engineer`): reads the spec and
the latest verdict, commits on the task branch, and ends each round
by writing `responses/engineer-round-N.json`:

    {
      "author": "engineer",
      "addressed": ["comment 1 -> commit X"],
      "pushback": ["comment 3 -> disagree because Z"],
      "notes": "free-form context for the reviewer",
      "followups": ["out-of-scope idea for a separate PR"]
    }

Reviewer (`spore-role/<project>/<slug>/reviewer-<instance>`): one
body, two instances, selected by `SPORE_REVIEWER_INSTANCE` (A or B).
A persists across its rounds and reads the engineer's responses. B
spawns fresh after A approves; round 1 is fresh eyes (no A history,
no engineer responses), rounds 2+ read only the engineer's replies to
B's own verdicts. Each round a reviewer writes
`reviews/<instance>/round-N.json`:

    {
      "author": "A",
      "verdict": "approve" | "request_changes",
      "summary": "one-line overall take",
      "comments": ["one string per concern"]
    }

## File validation

Every drive tick validates each role file before deriving the phase
(`internal/task/rolefile.go`). A file is invalid when it is not one
JSON object, carries a key outside its schema, lacks a required field
(`author` everywhere; `verdict` and `summary` on reviews), has a
verdict outside `approve` / `request_changes`, or has an `author`
that does not match its location: `reviews/A/` needs `"A"`,
`reviews/B/` needs `"B"`, `responses/` needs `"engineer"`. Panes fill
it from `$SPORE_REVIEWER_INSTANCE` / `$SPORE_ROLE`.

An invalid file never counts toward the phase, so it cannot wedge or
skew the loop. The drive tells the coordinator once per bad file and
wakes the authoring pane with the validation error. Once the wake is
confirmed, the file moves to `<name>.invalid` (then `.invalid.2`, ...)
for debugging and the pane rewrites it. If the authoring pane is not
running, the file moves aside at once and the coordinator notice is
the only signal.

Upgrade path: the first drive by a validating binary writes
`state/legacy-files`, listing the role files already on disk. Those
are read leniently (unknown keys and a missing `author` pass; the
verdict enum still applies), so an in-flight loop does not rewind.
Files written after that point are strict. Panes spawned before the
upgrade still run the old role body; their next file without
`author` is bounced, and the wake message names the missing field.

## Observing a running loop

`spore task status <slug>` prints the derived snapshot: phase,
engineer round, current reviewer, reviewer round, last verdict.

Markers under `.spore/<slug>/state/`:

- `woken-<reviewer>-<round>-<stamp>`: the engineer was nudged for
  this verdict. The stamp is the pane's tmux `session_created`, so a
  respawned engineer pane gets re-nudged on the next tick.
- `escalated-<reviewer>-<round>`: the loop escalated. Cleared when
  the loop later reaches done (operator force-approve).
- `ready`: the loop reached done; the branch awaits the operator.
  Cleared by `spore task done`.
- `legacy-files`: role files grandfathered from before validation.
- `respawn-<role>-r<round>-<n>`, `respawn-capped-<role>-r<round>`,
  `activity-<role>-r<round>`, `stalled-<role>-r<round>-<since>`: pane
  liveness (see above).
- `invalid-<file>-<n>-<hash>`: the coordinator was told about this
  invalid file; `bounced-<file>-<n>-<hash>-<stamp>` marks the
  delivered wake.

`spore task waybar` renders `d/a/p/b/e/r` counts (draft, active,
paused, blocked, escalated, ready). Escalated and ready count marker
files regardless of frontmatter status. The chip CSS class is the
highest-priority nonzero bucket: `escalated`, then `blocked`, then
`ready`, then `active`, else `idle`.

`summary.md` lands in `.spore/<slug>/` when the loop hits done: both
review threads (verdict, summary, comments per round) plus every
engineer response. It embeds a fingerprint of the artifact counts and
regenerates if the tree changes under it.

## Finishing

When the loop is done, review the branch (`summary.md` is the digest)
and merge with `spore task merge <slug>`. Then run:

    spore task done <slug>

Done flips the status, kills the homogeneous worker session, removes
the worktree and the `wt/<slug>` branch, deletes `tasks/<slug>.md`,
and removes the whole `.spore/<slug>/` tree (spec cache, responses,
reviews, summary and state markers), so a later task reusing the slug
starts its loop from round 1 with its own spec. It refuses while the
branch has unmerged commits (`--force` discards them).

Done does not kill role panes itself: its session cleanup matches
`<project>/<slug>` at a name boundary, which `spore-role/...` names
never hit. The next reconcile pass does. It reaps every
`spore-role/<project>/<slug>/*` pane whose task is done, deleted or in
an unknown state, the rule generic workers follow; panes of active,
paused, blocked and unparseable tasks are kept. On the normal path the
done tick already reaped them; an abandoned escalated task's panes go
on the first pass after its status leaves active for done.

## Spawn hardening in this fork

Role panes get the same spawn treatment as generic workers. Their
tmux session carries the project's `agent.env` entries after spore's
own `-e` flags (reserved `SPORE_*` and `WT_*` keys are dropped, as for
workers), and with `[worker] isolate_network = true` the agent runs
under `pasta`; a missing `pasta` refuses the spawn instead of running
the pane unisolated. A pane that dies within ten minutes of its spawn
waits out a backoff (1 minute, doubling to 30) before the next spawn.
The backoff is kept per pane, as `<slug>/<role>` (for example
`demo/engineer`; a loop step uses its session name), in `role-spawns.json` in the project state dir,
separate from the generic fleet's `fleet-spawns.json`. A pane that
stays healthy past ten minutes clears its record. The backoff applies
to both the silent respawn of a missing session and the counted
respawn of a dead agent; the `MaxRoleRespawns` cap is unchanged. Role
notices go to the coordinator inbox the watcher reads, resolved from
the project root and the state dir settings.

Role panes have no `SPORE_TASK_INBOX`; `spore worker token-monitor`
meters them through `SPORE_ROLE` at the worker wrap cap. Past it, the
role agent is told to commit in-flight work to the task branch, write
its step's output under `$SPORE_TASK_DIR` only if complete, and end
its own session (named by `SPORE_ROLE_SESSION`). If a token wrap or
anything else kills a role session, the driver respawns it silently
on the next tick.

## Loop files

The role loop is one loop file among others. A task opts into a loop
with `loop: <name>` in its frontmatter; `loop: role` stays valid and
means the `dev` loop, which reproduces the engineer and two-reviewer
flow above. Spore ships two loops in `bootstrap/loops/`: `dev` and
`example`, a minimal drafter and checker to copy from.

A project file `bootstrap/loops/<name>.toml` overrides the bundled
loop of the same name, by the same rule as `bootstrap/roles/<role>.md`.
A broken override (a dangling symlink, a directory) is an error, never
a silent fallback to the bundled loop.

The reconciler classifies every active task with a `loop:` key:

| `loop:` value | Reconcile does |
|---|---|
| empty | spawns the usual single worker |
| a name with a loop file | drives the loop, no generic worker |
| a name no loop file carries | warns and spawns a generic worker |
| a loop file that fails to load | warns with the file and line, starts nothing |

### Schema

Top-level keys: `name` (must match the file name), `summary` (write
`summary.md` on done: the dev loop's review summary, or for any other
loop one section per agent step with each round's output), `github_mirror` (run
the draft-PR mirror when `pr_flow = "draft"`; the mirror reads only the
dev loop's outputs, so the loop's agent steps must write exactly
`responses/engineer-round-{round}.json` (shape `engineer-response`) and
`reviews/A/round-{round}.json`, `reviews/B/round-{round}.json` (shape
`verdict`)) and `pr_footer` (default
`true`; `false` drops the "Dispatched by ... merges." footer, so a PR
body opened by `push-and-open-pr` or the mirror is the spec summary
only). Then one `[[steps]]`
table per step, in order:

| Key | Meaning |
|---|---|
| `name` | step name; `done` is reserved |
| `kind` | `agent` (a pane writes an output file) or `action` (spore runs a built-in) |
| `role` | role body `bootstrap/roles/<role>.md` (agent) |
| `session` | pane `spore-role/<project>/<slug>/<session>`; defaults to `name` |
| `title`, `label` | wake text name; label also names markers and `SPORE_REVIEWER_INSTANCE` |
| `phase` | phase name when the step is entered forward |
| `persist` | keep the pane alive after the cursor moves past the step |
| `output` | file under `.spore/<slug>/`, `{round}` expanded; may not leave that dir |
| `shape` | `engineer-response`, `verdict` or `json` (with `required = [...]`) |
| `skills` | skill names appended to wakes and exported as `SPORE_STEP_SKILLS` |
| `slot`, `mode`, `agent`, `effort` | start from `[fleet.reviewers.<slot>]`, then override |
| `max_rounds` | escalate instead of jumping back once the step has written this many outputs |
| `action`, `message`, `label` | action name and its parameters (action) |

`[[steps.done]]` adds conditions on top of the output: `kind = "file"`
(`path`), `"verdict"` (`equals`; the field is always `verdict`) or
`"command"` (`run`, `timeout`); only agent steps take them, and a file
that puts one on an action step fails to load. `[[steps.next]]` picks the next step by
verdict: `on`, `to` (a step or `done`), and `return = true` to come
back to this step once the target answers. Without a matching `next`,
the loop moves to the following step; past the last step it is done.
A verdict condition and a `next` on a different verdict cannot share a
step (that verdict could never route); loading such a file fails.

A step whose output exists but whose conditions do not hold is held.
A held round followed by a newer output of the same step is
superseded: the loop judges the newer round, and superseded rounds do
not count toward `max_rounds`. A round that has a newer output is
judged by its cached check results only: its check never reruns, so a
fix commit cannot pass the old round in place of the newer one. A step held by a file or verdict
condition is reported to the coordinator once per step and round
(`state/held-<step>-<round>`).

#### Command checks

A `command` condition runs `run` under `sh -c` in the task worktree
with `SPORE_TASK_SLUG` and `SPORE_STEP_OUTPUT` (this round's output
path) added to the environment, bounded by `timeout` (seconds or a Go
duration such as `"30s"`; default 600s). A result is cached in
`state/check-<step>-r<round>-<fp>.json`, keyed by a fingerprint of the
round's output bytes, the worktree HEAD and the command, so a check
reruns only when the output file or the task branch changes, never on
every tick. Once a round passes it stays passed. On failure the step
is held, its pane is woken once per fingerprint with the command
output and "Write round N+1 now: <path>", and the coordinator is told
once per fingerprint. A failed check does not count toward
`max_rounds`: it only means the step's work is not done yet. A
command condition needs the step to have an `output`.

#### Actions

Actions run once per visit: a visit is recorded as
`state/action-<step>-v<n>` after the action succeeds, and a failing
action parks the loop on its step until a later tick succeeds. Action
parameters sit flat on the step (`message`, `label`) or in a
`[steps.params]` table, which wins on a clash. The built-in actions:

- `notify-coordinator` (`message`, default "reached <step>") tells the
  coordinator "<slug>: <message>".
- `push-and-open-pr` (`label`, `draft = "true"` or `"false"`, default
  false) pushes the task branch as the project's GitHub App and, on the
  first visit that finds no PR in `github.json`, opens one (or adopts
  the PR already open for the branch); later visits only push. It
  needs `github.toml` with `operator_login` set.

No action and no driver path ever merges a PR.

Two reconcile passes never drive one slug at the same time: the drive
holds a non-blocking flock on `state/drive.lock`, and a pass that finds
it held skips the slug.

### Panes and wakes

The current step's pane and every `persist` pane at or before it stay
alive. When the cursor reaches a step forward, earlier non-persist
panes are reaped; a step that sent work back keeps its pane for the
re-check. Done reaps every pane; escalated touches none.

A step that is sent work, with or without `return`, is woken with
"`<title>` requested changes at round `<n>`" and the verdict path.
When the sender's turn comes back, it is woken with "`<title>`
responded to your round `<n>` verdict" and the new output's path.
Outside the dev loop (whose wake text is upstream's) both wakes end
with "Write round N now: <path>". Each pane gets each wake once
(marker `state/woken-...`). A pane the driver reaps on purpose does
not count as a quick death, so re-entering its step never waits
behind spawn backoff.

The loop exports `SPORE_LOOP`, `SPORE_STEP`, `SPORE_STEP_OUTPUT` (the
absolute output path, `{round}` unexpanded) and `SPORE_STEP_SKILLS` to
its panes, next to the role-loop variables above.

#### The step note

Role bodies are shared between loops and name the role loop's own
paths, so each agent step with an `output` also gets
`SPORE_STEP_NOTE`, put ahead of its role body in the first prompt
(after the pre-start failure line, when there is one). The note names
the step and loop, the absolute output template, the shape's exact
keys and the `author` a verdict must carry, and says these replace any
path or format the role body names. A pane spawned at the cursor is
told "Write round N now: <path>"; a persisted pane idling behind the
cursor is told its next round and to wait for its wake. A pane
spawned at round 2 or later (a re-entered step, a crash or token-wrap
respawn) has no other way to learn its round. Custom role bodies need
not know about loops.

### The example loop

    name = "example"

    [[steps]]
    name = "draft"
    kind = "agent"
    role = "engineer"
    persist = true
    output = "draft/round-{round}.json"
    shape = "json"
    required = ["summary"]

    [[steps]]
    name = "check"
    kind = "agent"
    role = "reviewer"
    output = "check/round-{round}.json"
    shape = "verdict"
    max_rounds = 2

    [[steps.next]]
    on = "request_changes"
    to = "draft"
    return = true

    [[steps.next]]
    on = "approve"
    to = "notify"

    [[steps]]
    name = "notify"
    kind = "action"
    action = "notify-coordinator"
    message = "example loop finished: the draft passed its check"

The drafter writes `draft/round-1.json`; the checker approves it or
sends it back (at most two rounds); the coordinator is then told.
