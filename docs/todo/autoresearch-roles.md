**Status**: built (2026-11-19). Lands on fork PR #59 (branch `autoresearch`). Operator doc: `docs/autoresearch.md`.

# Autoresearch for loop roles: the bug-replay judge

## Intent

The fleet now does most of its work through loops (`docs/role-loop.md`),
so the role bodies that drive each loop step matter more than any one
skill. The most important loop today is marketer-frontend's `qa-e2e`
loop (`bootstrap/loops/qa-e2e.toml` in that repo): qa-analyst,
qa-author, qa-reviewer, qa-completeness, qa-reporter.

The operator wants autoresearch (`docs/autoresearch.md`) to improve
those roles. "Better" means two things, in the operator's words:

- **Find more bugs.**
- **Write better code**: less fluff, closer to Clean Code, more
  understandable.

Autoresearch v1 can only target a `SKILL.md`, and its judge can only
run single-prompt scenarios. This change adds role targets and a judge
that measures bug-finding without a live stack.

## The core idea: replay bugs we already know

We cannot run e2e tests in the judge sandbox (no backend, browser or
Vite). Instead we replay history. Each scenario is one real, already
known bug:

1. The agent under test gets what it would have had BEFORE the bug was
   known: the feature ticket, the product knowledge for the feature,
   and the code as it was before the fix.
2. It does its normal job (a test plan, a spec).
3. A grader, which alone sees the bug's steps to reproduce, decides
   whether the agent's output would have tripped the bug.

The bug corpus uses both sources (operator, 2026-11-19):

- bugs our QA workers filed (skill `linear-bug-report`), which show
  what the agents already catch;
- bugs from customers and developers, which the `assistant`
  coordinator files, which show what the agents miss.

## Goals

1. `spore research new` accepts a role body as a target, not only a
   `SKILL.md`. The editor may change only that file.
2. A role judge: runs the candidate role body as the agent's opening
   prompt inside the existing bwrap sandbox, with the fixture
   environment a loop step would give it, and scores its output.
3. Bug-replay scenarios: the scenario format can express "feature
   ticket + knowledge pages + pre-fix code snapshot in, plan or spec
   out, graded against hidden repro steps".
4. A corpus builder: `research judge draft` for a role target mints a
   worker task that mines Linear for usable bugs and writes them as
   scenarios (main and holdout sets).
5. A code-quality score for qa-author output: no fluff, Clean Code,
   understandable, reuses existing page objects and helpers.
6. Holdout fixes (operator, 2026-11-19):
   - a winner must BEAT the original on the holdout, not tie it;
   - the cached holdout result must be invalidated when the judge
     changes (deferred finding `holdout.go:125`).

## Non-goals

- Coordinator role files. They are long-running and event-driven; one
  scenario run cannot judge them fairly.
- Real execution of generated specs (approach B below). It is a later
  confirmation step, after several campaigns, not part of this change.
- Running a whole loop as the judge (approach C).
- Any change to the loop engine (`internal/fleet`, `bootstrap/loops`).
- Scheduling nights. They still start by hand.

## Anti-goals (would be a failure even if it works)

- **Leakage.** If the agent under test can see the bug ticket, the fix
  commit, or code from after the fix, the score measures recall, not
  testing skill. Every scenario must be built so the agent cannot reach
  them: the fake Linear service serves only the feature ticket, the
  code snapshot is the fix commit's parent, and git history is not in
  the snapshot.
- **Rewarding fluff.** A plan with 200 scenarios trips every bug and is
  useless. The score must punish bloat, or the loop will learn to pad.
- **Customer data in the repo.** Corpus content (ticket text, customer
  names, repro steps) never gets committed to spore, which is
  open-source bound. It lives in the git-ignored `research-drafts/`
  and the campaign directory, as judge sets do today.
- **A grader that cannot be checked.** The grader's verdict must quote
  the scenario it matched against the repro steps, so a human (and
  later approach B) can audit it.

## Approaches

- **A. Static replay with a grader (chosen).** The agent runs once per
  scenario in the sandbox; an LLM grader with a strict checklist
  decides "would have caught it"; checks and a rubric score code
  quality. Cheap, fits the overnight loop. Risk: the grader's call is a
  judgement.
- **B. Real execution.** Run each generated spec against a backend and
  frontend at the pre-fix commit and see it fail on the bug. Exact, but
  needs a live stack per run outside the sandbox; a night would fit a
  handful of tries. Operator: keep B as a later confirmation step,
  after several cycles of A, to check the grader against reality.
- **C. Whole loop as judge.** Most realistic, hours and real money per
  scenario. Rejected.

The operator chose A on 2026-11-19, with B as a later confirmation.

## Design

### 1. Role targets

- `spore research new <c> --target <file> --kind role --step <name>`.
  `--kind skill` stays the default and keeps today's `SKILL.md` rule.
  A role target is any `.md` file. `--step` names the loop step whose
  role this is; the judge reads that step's `output`, `shape` and
  `skills` from the target project's loop file at `new` time and pins
  them in the campaign (a later edit to the loop file does not change
  a running campaign).
- The editor may change only the target file. Frozen context it may
  read: the loop file, the step's skills (copied at `new`), `goal.md`
  and `results.tsv`, as today.
- `accept` for a target inside ANY git repo (not only spore's) commits
  the winner on branch `research/<campaign>` through a temporary
  worktree, never touching the checkout. Outside a git repo it keeps
  today's overwrite-with-backup. The role bodies live in
  marketer-frontend, so their winners arrive as a branch there, ready
  for a bot PR with the `QA` label.

### 2. The role judge

`spore eval judge` gains a role mode. Per scenario run it builds a run
directory that looks like a loop step's world:

- `$SPORE_TASK_DIR/spec.md`: the scenario's task spec (what the
  coordinator would have written for the feature).
- the step's skills under `.claude/skills/`, frozen copies from `new`.
- the code snapshot (see 3) as the working directory.
- fake MCP services (existing `fakemcp`) for Linear and Notion that
  serve only the scenario's allowed tickets and pages.
- env as a role pane gets it: `SPORE_TASK_DIR`, `SPORE_ROLE`,
  `SPORE_STEP_SKILLS`, plus the step note naming the output path and
  shape, prepended to the candidate body exactly as the loop does.

The agent's prompt is the step note plus the candidate role body, then
"begin". It runs once, headless, under bwrap, with today's caps.

### 3. Bug-replay scenarios

A role scenario directory holds:

- `scenario.json`: existing fields plus `kind: "role"`, the step, the
  allowed services, and checks.
- `spec.md`: the task spec given to the agent.
- `snapshot/`: the code the agent may read, taken at the fix commit's
  PARENT. Sparse: the files the feature touches plus what the role's
  skills tell it to read (playwright/, page objects, seeds, mocks).
  Both repos (marketer-frontend and marketer) when the feature spans
  them. No `.git` directory.
- `grader/`: visible to the grader only, never mounted for the agent:
  the bug's repro steps, expected vs actual, the bug id.

Checks per scenario:

- `output-valid`: the step output exists and has the step's shape.
- `bug-caught` (LLM grader): "Does any scenario or test in this output
  exercise these repro steps closely enough that running it would have
  failed on this bug?" Answer yes/no plus the quoted scenario that
  matches. A yes without a quote counts as no.
- `fluff` (role-dependent):
  - qa-analyst: scenario count within a cap derived from the feature
    (the corpus builder records the count of a reasonable plan; the cap
    is 1.5x that), no duplicate scenarios (grader-judged).
  - qa-author: the code-quality rubric below.

A scenario passes when every check passes. The campaign score stays
the pass rate (0-100), so the research core is unchanged.

### 4. Code-quality rubric (qa-author)

Mechanical checks first (cheap, exact), each a regex or file check on
the spec the agent wrote:

- no added comments in spec files (repo rule, skill `new-test`);
- no assertion that only checks an element is visible or exists
  without asserting an outcome;
- no new helper or page-object method that duplicates one present in
  the snapshot (grep by name and by body similarity);
- end-state asserted through the UI, not through an API response.

Then one grader rubric, 0-3 each, pass at a fixed threshold: names say
what they do; one level of abstraction per test; no dead code or
unused setup; a reader new to the feature can follow the test.

### 5. Corpus builder

`spore research judge draft <c>` on a role campaign mints a worker task
(draft, not started) whose brief:

- mines HK bugs in Linear from both sources (QA-filed and
  assistant-filed), keeping only bugs with: a linked feature ticket,
  usable repro steps, and an identifiable fix (a merged PR or commit),
  so the pre-fix commit is known;
- for each kept bug, writes one scenario (spec, snapshot, grader
  folder) and records the pre-fix commit, so approach B can replay it
  later;
- splits by bug into main and holdout, holdout at least 30% and at
  least 5 bugs, and keeps bugs of one feature on the same side;
- proves discrimination: a deliberately weakened role body (for
  example, with its bug-hunting instructions removed) must score lower
  than the current one on the main set;
- reports which bugs it dropped and why.

The operator reviews the drafts before `judge approve`, as today.

### 6. Holdout changes

- **Beat, not tie.** The winner passes only when its holdout score is
  strictly greater than the original's (`holdout.go:68`). If the
  original already scores 100 on the holdout, the verdict is
  `no-headroom`, and accept refuses it unless `--force`. Lower bound
  on holdout size moves from 1-3 scenarios to at least 5.
- **Cache key.** The per-side holdout cache (`holdout.go:125`) is keyed
  by commit plus HoldoutHash, BinaryHash and the judge argv. A changed
  judge discards the cache.
- **Headroom check before night 1 (my call).** If the baseline scores
  100 on the main set, `night` refuses with `no-headroom`: there is
  nothing to improve and the night would only spend quota.

### 7. Later: approach B as confirmation

Not built now. The corpus records each scenario's pre-fix commit, so a
later `research calibrate` can boot a stack at that commit, run the
winner's generated spec, and compare reality with the grader's verdict
on a sample of bugs.

## Constraints

- Lands on PR #59's branch (operator, 2026-11-19), built by spore
  workers through a dev loop, test-first (repo CLAUDE.md).
- Corpus content never committed (see anti-goals).
- The production QA roles run on Opus. A campaign may run on haiku to
  save quota, but a winner tuned on haiku may not carry over. The model
  is per campaign; the operator picks it.

## Left to the builder

- Exact flag names and file layout inside a role scenario.
- How the step note is reproduced (shared code with `internal/fleet`
  is fine if it does not import fleet into evalharness; a copy with a
  test that pins them equal is also fine).
- The similarity method for the duplicate-helper check.
- Snapshot trimming rules beyond "what the feature and the skills
  touch".
