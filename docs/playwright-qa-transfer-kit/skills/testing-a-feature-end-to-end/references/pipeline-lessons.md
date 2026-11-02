# Pipeline lessons

Per-stage rules for the `testing-a-feature-end-to-end` pipeline. Each lesson is stated as a rule. Lessons
marked **ANTI-LESSON** describe a mistake that recurs when the whole arc is run; they are the highest-value
items. Read the stage section before running that stage.

---

## 1. Intake / ticket analysis

- **Split the editable QA ticket from the read-only developer ticket; never wire the developer ticket's id
  into branches or PRs.** The developer's feature ticket stays untouched; an operator-owned QA ticket holds
  all analysis and links to the developer ticket as a passive relation. When filing a related fix, keep the
  developer ticket's identifier out of the branch name, PR title and body, so the {{TRACKER}} integration
  cannot mutate it.

- **Start with a dedicated analysis worker that produces a behavior surface + test plan before any code.**
  Surface a fan-out go/no-go to the operator rather than auto-fanning-out.

- **ANTI-LESSON - verify WHERE the specs actually live first.** The ground-truth spec set is usually the
  remote PR tip, not `{{DEFAULT_INTEGRATION_BRANCH}}` or a stale local branch. Check the remote head before
  you review.

## 2. Product intent (intended behavior = oracle)

- **Docs / {{INTENT_SOURCE}} first, not code, for "how should this behave."** Authority model:
  {{INTENT_SOURCE}} = how it *should* behave (intent); code = how it *does* behave (ground truth). On
  conflict, FLAG the gap; follow neither silently.

- **Use the intent source to classify bug vs extension, and cite the mechanism.** A "Bug" label on a ticket
  is a symptom, not a cause; a change can be an intended extension of an existing flow. Ground the
  classification in the documented origin of the behavior.

- **ANTI-LESSON - a `test.fixme` oracle must trace to a documented requirement.** An expectation that traces
  only to your own QA note (and that the intent source calls spec-silent) is a product-owner question, not
  an acceptance guard. Re-frame it as a "PO ruling needed" item instead of shipping it as a failing guard.

- **Verify scope questions against backend git history.** "Is sub-feature X part of this deliverable?" ->
  check whether its code landed in the same commit as the feature.

## 3. Test planning & scope (consolidation philosophy)

- **Merge cases that share preconditions AND steps into one carrying multiple asserts.** The more a single
  test verifies, the higher its value. Consolidate with zero asserts dropped.

- **ANTI-LESSON - no standalone "the list/page loads" test.** In any test that loads the list, first assert
  on it through a page-object function that takes the expected list and asserts visibility. It costs almost
  nothing and reduces flakiness. Delete the standalone load test.

- **Merge tests that share the same seed + setup - do all asserts in one test.**

- **ANTI-LESSON - stop at the "safe floor"; do not over-merge.** Reviewers' merge counts can double-count
  merges their own rules refuse. Going past the constraint-honoring number reddens the suite on the first
  fix of a filed bug. Never merge a blocked assert with an unblocked one, or across auth / seed-state /
  destructive-step / boot-env boundaries.

- **Merge-vs-keep rule of thumb:** merge when same seed + auth + page + sequential action AND both unblocked
  AND the failure signal stays legible; keep separate if any of auth / seed-state / destructive-step /
  boot-env / blocked-by flips; blocked asserts cluster only with same-blocker asserts.

- **Co-locate rather than merge into one test** when two specs share a seed but one is a passing
  characterization and the other a failing `test.fixme` oracle (merging would break `@smoke` or kill the
  fix-acceptance signal). Move both into one file instead.

- **Delegate a heavy matrix refactor to a subagent, but require an old -> new mapping + a before/after
  assertion-count integrity check** (not the whole document - protect context), and forbid entangling
  blocked with unblocked assertions.

## 4. Backend + seed + mock setup

- **ANTI-LESSON - there is NO shared e2e backend.** Every consumer provisions its OWN per-slug backend
  (`{{BACKEND_WORKTREE_ROOT}}-{{SLUG}}`, own DB/cache/port). A leftover shared stack is an orphan to tear
  down. Use SHORT task slugs - a long slug pushes the Postgres unix-socket path past 107 bytes and the
  backend will not boot.

- **Prepare the test DB with `db:schema:load` (not migrate); verify the expected schema state before
  running.** See `recipes.md`.

- **Additive, flag-gated seeds as SEPARATE keys** so existing list/filter assertions stay byte-identical. Do
  NOT enable a behavior unconditionally for every seed: it regresses unrelated specs (extra async fan-out
  saturates the job pool; id reuse after a clean causes cross-test contamination). Use an opt-in seed flag
  and isolate the specs that need it in their own `describe`.

- **Cross-repo declare -> ack, but the worker's OWN per-slug backend worktree on its companion branch is NOT
  "shared"** -> author, commit and push there freely. Only SHARED checkouts (`{{BACKEND_CHECKOUT}}`,
  `{{MOCKS_HOME}}`) keep the declare -> ack rule. For a SHARED seed file, keep a single-integrator model.

- **GOTCHA - a factory trait can assume a vendor-specific error shape.** If a "failed" trait decorates the
  error with vendor metadata and breaks on a plain error, bypass the trait: create the record plainly and
  call its state transition with a plain error.

## 5. E2e authoring (Playwright, Page Object Model, no comments)

- **ANTI-LESSON - over-commenting. HARD RULE: FLAT BAN on comments in test code.** Typical sources of comment
  bloat: provenance bleeding into source (backend `file:line` and SHA pins copied into spec comments),
  orphaned demo/dead code with heavy JSDoc, and blanket "JSDoc every method" plus next-line paraphrase of the code. Add
  NO comments to spec, page-object, fixture or seed files. Rename or restructure instead. Provenance goes in
  the PR description or the ticket; conventions go in the folder `CLAUDE.md`. No exceptions. Resolve a comment-vs-comment merge conflict by
  deleting the comment.

- **Recover deleted PO/spec scaffolding from git** (`git show <sha>^:path`) rather than re-authoring.

- **Expected-fail guards are static `test.fixme('...', fn)`** whose oracle encodes the intended end state,
  so it flips to pass exactly when the blocking ticket is fixed.

- **Assert list content via a reusable PO helper that takes the expected list as an argument**
  (a `validate<Thing>List(expected)` pattern).

- **ANTI-LESSON - a server-driven page that navigates via Turbo (or any client-side visit) makes
  `waitForLoadState` a no-op.** Gate on the destination control / page indicator after a link click, never a
  load-state wait or a sleep. Symptoms when ignored: a fill lands in a same-named field on the outgoing page
  mid-transition; pagination reads a stale page 1.

- **Partition multi-worker authoring on ONE branch by page-object ownership** (one sole editor per PO
  file); hold cross-PO tests as a coordinator phase 2.

## 6. Assertion depth

- **Never stop at "the button/link exists."** Click the link and assert the correct page opened; cover all
  CRUD, not just Read; exercise flows to completion and assert their outcome with in-depth checks.

- **A state-changing action has MULTIPLE effects - assert them ALL.** Especially the state change AND the
  outbound {{VENDOR_A}} request (via the mock transaction-log helper). Compute the FULL expected set up
  front, then assert each item reached its end state AND produced its matching outbound request (matched by
  name + destination).

- **The completion oracle for an async fan-out is "all expected effects observed," not a timer or a
  sample.** Read the job executor's counters; block until the queue is empty and scheduled == completed;
  RAISE on timeout (no false green).

- **Assert only what the UI actually renders.** A field that no page shows is not a UI assertion target;
  assert the rendered consequence on the real page instead.

- **For a conditional branch, the NEGATIVE assertion (branch-leak guard) is the real value.** The positive
  half is often already covered by the list render.

## 7. Run / CI + failure triage

- **ANTI-LESSON - do not hand a STATIC root cause off as a live repro.** If a conclusion rests on static
  analysis, say so and reproduce it in isolation. The isolated per-slug backend exists precisely so you can.

- **Classify EACH failure individually - one fix rarely clears them all.** After a fix, remaining failures
  can split into the fixed cause, infra/timeout flakes, and one unrelated real failure that a "one cause"
  assumption would miss.

- **Frontend e2e CI runs `{{CI_BACKEND_ARTIFACT}}`, not the PR's backend;** its entrypoint loads the schema.
  A stale schema file on the backend's integration branch breaks ALL PRs' e2e. Triage by diffing the schema
  file against the migrations folder (a missing column -> 500 across unrelated PRs).

- **ANTI-LESSON - "green in isolation, flaky in the full local run" has a deterministic trap.** Before
  calling it a race, rule out: the local stack missing an opt-in flag (the action no-ops -> consistent
  timeout), and an ungated live external dependency in the code path. Prove the latter by booting the
  backend with that dependency's URL pointed at a dead address: an identical failure signature confirms it.

- **ANTI-LESSON - "full run" means the whole folder, not the one file you are iterating.** A shared-seed
  change (for example moving a timestamp outside a time window a backend job enforces) silently breaks a
  sibling spec.

- **ANTI-LESSON - reconcile every claim against PRIMARY logs; correct wrong hypotheses.** A coordinator's
  parse of a trace can be wrong and get handed to a worker as ground truth; force an independent re-read of
  the raw trace / network log. One symptom can have multiple independent causes.

- **PR e2e failure runbook = PARALLEL:** kick ONE CI retry per SHA AND dispatch a local-repro worker at the
  same moment; green = silent, red again = escalate. Max one retry per SHA.

## 8. Bug hunting

- **A bug found while writing tests is a SUCCESS - surface it, never work around it to go green.** Surface
  it as a `test.fixme` with the intended end state (typical finds: an unhandled empty input -> HTTP 500; a
  state-mutating GET; a status badge that is written once and never cleared).

- **Split a false green** into a labelled CHARACTERIZATION test (present buggy behavior, green) + an
  intended-end-state `test.fixme` - never leave a buggy assertion reading as the intended contract.

- **ANTI-LESSON - the MAJOR workaround.** Use the flows the feature provides, from a user's point of view.
  A seed/transition script that runs a job synchronously in place of clicking the real button is a major
  workaround; rework it to click the real button.

- **ANTI-LESSON - even an operator's own same-day instruction can be wrong; FLAG, do not silently comply.**
  If you can prove an instructed approach breaks behavior (for example a drain that double-processes under
  the real async adapter), surface the gap with the proof instead of overriding unilaterally; let the
  operator choose.

- **Distinguish a product bug from an e2e-environment blind spot.** The latter is unfilable and warrants a
  justified `test.fixme`, not a ticket. Examples: an in-process async job adapter never populates the real
  queue, so any "already running" UI state is unreachable; a review-app sign-in fallback that auto-signs-in
  an admin makes access denial untestable in the UI (cover it in backend specs).

- **Keep the fix in TEST code; if the real fix is app code, DOCUMENT it for developers, do not implement
  it.** A test-gated branch in product code violates `qa-e2e-test-only-changes`; put the substance in the
  report as a SUGGESTED DEV IMPROVEMENT.

- **A genuine root cause can be test-only-fixable AND expose a product gap - surface both.** For example a
  seed that picks a random enum value (`.sample`) fails exactly on the draws that hit an unsupported value:
  pin the value in the seed, and report that the product lets users select a value that can never succeed.

## 9. Open PR

- **Push to the working branch is ALWAYS allowed (keeps the draft PR current); only MERGE is
  operator-gated.** Do not conflate push with merge.

- **Merging to a persistent branch is NEVER the coordinator's or worker's job.** Only the operator merges,
  manually.

- **Push recipe:** under `{{SECRETS_WRAPPER}}` inside `{{DEVSHELL}}` (so the pre-push hook has its
  toolchain); commit as `{{AUTHOR_IDENTITY}}` via `git -c` overrides; keep the PR DRAFT; verify the remote via
  `git ls-remote` + `gh pr view`. See `recipes.md`.

- **A one-shot backend change** goes in a THROWAWAY worktree off the backend's integration branch: commit,
  push, open a PR (not draft), remove the worktree - never touch the shared checkout. **Bring a stale PR
  branch current** by merging the integration branch into it via a throwaway worktree (so already-merged
  changes stop showing as diffs) - still never merge INTO the integration branch.

## 10. Quality loop

- **Run an INDEPENDENT adversarial senior-QA coverage review (NOT the suite's author).** Use agent
  `senior-qa-e2e-reviewer` / skill `reviewing-e2e-coverage`. It grades assertion quality
  ([Presence-only] / [Shallow] / [Deep]), checks full CRUD + flow execution + negative/edge, and outputs an
  actionable report another worker can implement (WHAT / file / PO helper / concrete assertion / seed
  prerequisite per item).

- **ANTI-LESSON - verify a review's own claims against the live tree.** Reviewers double-count merges and
  claim files are absent that the live diff shows present. Trust the tree, not the review.

- **A cleanup pass greps every flagged "dead" method for spec references FIRST** and splits truly dead from
  live-but-flagged before deleting.

- **After a behavior-preserving change, re-run for IDENTICAL pass/skip counts** to prove zero behavior drift
  before pushing; carry an old -> new test map for every consolidation (asserts in == asserts out).

## 11. Interaction & branch completeness check

- **Dispatch this as an INDEPENDENT worker (not the spec author)** - same independence discipline as stage
  10's adversarial review, applied to raw coverage rather than assertion quality. Three checks:

  1. **UI interaction completeness - every element's full designed interaction set, not click-only.** "Was
     this button clicked" is the wrong question if the element's design supports more than a click. Read the
     component's own handlers (`onClick`, `onChange`, `onDrag*`, a functional `onMouseEnter` / hover-reveal -
     not a decorative CSS `:hover`, `onKeyDown`, ARIA `role`) to find the real interaction set it was built
     for: click, type/fill, select-from-dropdown, drag, hover-reveal, checkbox/toggle, keyboard
     activation/shortcut, right-click/context menu, multi-select. Cross-check that each distinct interaction
     is the target of a matching Playwright action somewhere in the suite - a `.click()` on a drag handle
     does not cover the drag.

  2. **API/GraphQL field completeness - for a ticket with no UI surface, or a mixed one.** Calling the
     endpoint once proves nothing about the fields. Enumerate every request field (required AND optional),
     every response field, and every distinct error/status path a changed endpoint or mutation can take, and
     cross-check each is exercised somewhere (a request spec, an e2e flow that triggers that specific
     field/error path, or a recorded manual check - see the automation note below).

  3. **Code-branch coverage.** Use the repo's existing Istanbul instrumentation - do not invent a new
     mechanism. In the reference project, `vite-plugin-istanbul` (`vite.config.ts`) instruments the frontend
     build when `VITE_COVERAGE=true`; the Playwright fixtures teardown (`playwright/support/fixtures.ts`)
     reads `window.__coverage__` and writes one JSON per test into `.nyc_output/`; `nyc` + `.nycrc.json`
     are in the repo. Boot the per-slug frontend dev server with `VITE_COVERAGE=true`, run the full relevant
     spec folder, then `nyc report --include '<changed src files>' --reporter=text` SCOPED to only the
     feature's changed files - scoping to all of the source tree dilutes real gaps into noise. Flag any
     branch/line inside a feature-relevant conditional at 0% or partial hit.

- **GOTCHA - an all-zero coverage report can mean "not instrumented," not "not covered."** Before trusting a
  0% result, confirm `window.__coverage__` was actually populated (the fixture logs per test whether coverage
  was collected or absent). If the dev server was not booted with `VITE_COVERAGE=true`, every branch reads
  0% and the check is meaningless, not damning.

- **Exhaustive enumeration != exhaustive automation.** The enumeration itself (checks 1 and 2) must cover
  EVERYTHING the feature exposes - do not sample or skip items to save time. Whether a given gap gets an
  automated test is a separate, deliberate call: state-changing / correctness-critical / regression-prone
  gaps go back to senior QA for a real test; a purely cosmetic or low-risk gap (exact hover-tooltip wording,
  an optional param with no branching logic behind it) may close instead with a documented "manually
  verified: what + how" note. The note must actually exist and name what was checked - an item with neither
  a test nor a note is still an open gap, not a judgment call already made.

- **Gate: any gap that is not closed by either a test or a manual-check note goes back to the senior-QA
  agent** (`senior-qa-e2e-reviewer` / `reviewing-e2e-coverage`) to decide and act, not just filed as a
  report that sits unaddressed. Loop until every enumerated item is accounted for, or the operator
  explicitly accepts a documented exception (e.g. a provably dead defensive branch, or a permission-gated
  element this feature's tests cannot reach).

- **Known blind-spot categories worth specifically probing for** (bugs that intent-driven QA tends to miss
  and live-data review tends to find):

  1. **Null/absent-field combinations in list rendering.** A list-row action conditioned on
     `field_a.present? && field_b.present?` can silently render NOTHING for rows where either field is
     NULL, instead of a disabled state or a fallback. Test the row shape your seed data does not usually
     produce, not just the happy-path row.

  2. **All-items-filtered boundary states.** When a default filter can hide 100% of a result set, the count
     header / empty-state wording can read as broken even though the data is correct. Test the "everything
     is filtered out" case explicitly, not just "some are."

  3. **Cross-consumer contract drift.** When a shared URL/param-generation helper changes, every OTHER
     consumer of that helper (e.g. a background job parsing the same URL) can silently break even though the
     page under test looks fine. When a feature touches a shared generator/service, grep for every other
     caller and check it against the new contract.

## 12. Final report

- **When the task is a review, the deliverable is an actionable report, not source edits** - report-only
  (no push/PR/merge; all tickets read-only).

- **Use a fixme taxonomy in the report:** (a) fix-acceptance guards for already-tracked developer gaps; (b)
  confirmed product bugs (to file); (c) unsourced expectations needing a PO ruling; (d) e2e-environmental,
  unfilable.

- **Every functionality/intent claim carries a verified source link** ({{TRACKER}} / PR / code permalink /
  {{INTENT_SOURCE}} / demo); flag anything that cannot be verified rather than asserting it.

## 13. Bug reporting

- **File verified bugs directly - do not wait for operator approval.** Parking a verified bug in a report or
  a chat message waiting for a blessing is the failure mode. Duplicate-search first, assign per the team's
  convention, put it in the active cycle. Use `{{BUG_FILING_SKILL}}`. "Verified" means Steps -> Actual ->
  Expected from something measured or observed, not inferred - an unverified suspicion is a finding to
  report, not a ticket to file.

- **When "expected" is not backed by a documented requirement, lead the note with a bold "PO ruling
  needed:" ask, then at most one line of context** - do not bury the ask.

- **ANTI-LESSON - once the operator rejects or holds a decision, NEVER re-raise it** (especially via a
  question prompt); execute the standing directive instead.

---

## Cross-cutting (applies across all stages)

- **Coordinator holds intent + verification; workers/subagents do the heavy read/write.** Every deliverable
  is coordinator-verified (`git ls-remote`, `gh pr view`, `git diff --stat`) BEFORE it reaches the operator
  or a ticket.

- **Verify before asserting - never state how infra behaves without checking live.** "Did the backend PR
  add seeds?" -> verify, then answer. "Did you reproduce?" -> honest yes/no.

- **A greyed autosuggestion / stale pane draft is NOT operator intent** - distinguish it from a genuine
  operator-typed draft (novel, forward-looking, references current state).

- **Workers self-wrap at token caps mid-task** - write a resume block to the task file, leave the
  environment UP, so a fresh session continues from evidence (verify, do not redo). The run-state
  checkpoint in `SKILL.md` operationalizes this.

- **Dispatch to an idle worker via the send-keys recipe** (C-u -> `-l` literal -> Enter); a `tell` may drain
  without re-prompting, and the TUI can swallow a bare Enter, leaving a dispatch UNSENT. See `recipes.md`.

- **Persist a durable lesson at MULTIPLE layers** so it reaches workers, not just the coordinator:
  worker-loaded skill rules + persistent agent memory + brief templates (+ checked-in repo rules, which is an
  operator-gated edit).

- **On ANY conflict - code vs intent source, or evidence vs an operator's own prior instruction - FLAG the
  gap and surface it; never silently follow either side.**

- **Fewer tests is a first-class quality goal** (less flakiness, lower run time) - but always with zero
  coverage loss and an explicit old -> new mapping.
