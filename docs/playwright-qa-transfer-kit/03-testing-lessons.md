# Testing lessons from running a production e2e suite

These are the generic rules a QA team learned while running the reference Playwright suite for a long time. Each one was paid for with a real incident; the incidents are left out, and only the rule and its reason are kept. They apply to any project that adopts the structure in `01-transfer-prompt.md`.

Use this doc in two ways:
- **Before a phase:** skim the section that matches the phase (assertions when authoring, isolation when booting stacks, CI forensics when CI goes red).
- **In review:** each rule is phrased so a reviewer can check a PR against it.

---

## 1. Assertion design

1. **Assert the rule, not the row.** If the test supplies a value (fill, factory, seed) and the field is system-derived (integration, URL template, precedence, fallback), a round-trip assert cannot catch the system computing it wrong. Derive the expectation from the documented rule. A round-trip is the right assert only where user input *is* the feature (rename, custom URL, free text).
2. **Every acceptance criterion gets a behavioural assertion,** including the ACs of tickets the change links to. Every AC that names a population ("applies to premium accounts") gets its complement tested ("standard accounts are unaffected"). If no seed can produce the complement, extend the seed.
3. **A test with no demonstrated pre-fix failure is superficial,** and a known-superficial test is deepened now, never shipped or deferred. Prove red against code carrying the bug before landing. Split a test until each rule fails on its own: an assertion behind an earlier one that aborts on the bug never ran. Where this rule collides with the complement rule, this one wins.
4. **One action, many effects: assert them all.** The primary state change (as the user sees it) and the outbound integration call are both mandatory. A banner or toast is never sufficient alone.
5. **Read the fixture, not just the assertion.** A seed or mock that only produces the happy variant makes the assertion structurally incapable of catching the bug. A failure mode the fixtures can't produce is a fixture gap to close.
6. **A synthetic fixture built for maximum distinguishability inflates blast radius.** It makes wrong output look worse than it is for real data. Re-measure severity on a realistic record before escalating.
7. **Before calling screen behaviour a bug, check the entry condition.** The seed may have built a state the product can't reach through its own flows.
8. **Derive formatted expectations in the browser** (dates, prices, phone numbers) with the page's own locale. A hard-coded string encodes the runner's locale and breaks on another machine.

## 2. Driving the app

9. **Drive the UI like a real user, with as few workarounds as possible.** Click the real action, drain async work through the one sanctioned mechanism, then poll the UI. Never replace the action under test with a backend shortcut. An unavoidable workaround is minimised and its reason goes in the PR.
10. **The rendered UI is the oracle.** Never assert a renderable value through the app's own API response or a DB/console read. The only legitimate non-UI asserts are outbound third-party calls (at the mock's log) and values with genuinely no UI surface. The app's own API may be used to navigate or set up.
11. **Some apps scroll an inner container, not the window.** Then `fullPage` screenshots return only the viewport and `window.scrollTo` is a no-op. Use the mouse wheel over the container or scroll the container element.
12. **A Suspense or loading fallback can render a duplicate of the app chrome.** Assertions on the sidebar or header can pass against the fallback before the real page exists. Scope chrome assertions to the resolved page.
13. **A missing frontend env var can blank a page and look like a product regression.** Boot locally with exactly CI's frontend env.
14. **"The login page renders nothing" is usually a leftover auth token** in browser storage, not a selector bug.
15. **Auth-helper gotchas.** The sign-in endpoint may answer 201, not 200. A reused API request context keeps the session cookie, so a later "does this password still work" check succeeds with any password. Use fresh contexts and clear cookies before credential checks.

## 3. Suite structure and code

16. **Consolidate tests that share seed and setup.** Seeding, sign-in and navigation are the slow, flaky part. No standalone "page loads" test: assert list content inline at the top of a functional test through a reusable page-object helper.
17. **No comments in test code.** Prose goes to the PR, the ticket or the folder `CLAUDE.md`. The ban is flat: no exception for any kind of comment.
18. **Test code must not import app runtime barrels.** One bad import kills the whole import chain, and Playwright reports "No tests found". Keep suite-own, dependency-free util leaves.
19. **Settle any Playwright API question from `node_modules`** (the installed version's `.d.ts` and lib), not from memory or a reviewer's recollection.
20. **Code review flags seed, mock or test changes made only to pass an assertion.** A fixture bent to match the code is a rigged test.
21. **Surface bugs; never make a test green by matching buggy behaviour.** Use `test.fixme` plus a finding or ticket.
22. **QA does not edit product code to pass a test.** Document the app fix for developers and keep the QA change test-side.
23. **A long-skipped spec may assert behaviour the product deliberately changed.** Date the skip against the behaviour-change commit before calling the failure a bug.
24. **Prove a pre/post-fix run against committed bytes.** Compare blob hashes, not file timestamps, and re-run after any edit.
25. **Prefer structural enforcement over a flippable flag.** A self-scoping project or a lint rule beats an env toggle plus documentation.
26. **Per-folder `CLAUDE.md` files work only if they are prescriptive at the decision point.** State the rule, not the incident, and back the most important rules with a lint.

## 4. Isolation and runtime

27. **Two runs sharing one backend truncate each other** through the per-test reset. Prove no other run is live before starting one.
28. **Workers that independently pick the same "free-looking" port corrupt each other.** Derive ports from a unique per-worker key.
29. **A killed run can leave in-process async jobs that deadlock the next run.** Restart the backend after killing a run. **Editing files under the dev-server root mid-run** triggers hot reload and destroys the page context of the running test.
30. **In-process async job adapters saturate under sleepy fan-out jobs,** and late jobs cross-contaminate the next test after id reuse. Gate fan-out per test and drain each fan-out.
31. **An in-process job adapter makes "job running" UI states unreachable.** Mark such a test `fixme` with the reason; never green it by asserting presence.
32. **For multi-service e2e, write down which services are under test, which are mocked, and the sync/async model before briefing anyone.** An unclear system-under-test boundary produces tests that pass against the wrong thing.

## 5. CI forensics

33. **"Skipped" checks are not green.** A gated job that skipped never ran its specs.
34. **CI tests the PR merge ref, and a rerun replays the stale recorded merge SHA.** A fix that landed on the base branch needs a fresh run (close/reopen, or a new push).
35. **A floating backend image tag can redden a frontend PR whose diff is inert.** Pin the backend artifact. Triage a red release by A/B-ing the backend image digest.
36. **An early-stopped shard (`maxFailures`) poisons run-to-run diffs** with fake "passed -> not run" blocks. Reconcile against the planned test list before comparing runs.
37. **The merged report decodes per-spec outcomes per attempt, and the failing spec can swap between attempts.** A re-run overwrites older attempts' artifacts; a published per-attempt report (for example on GitHub Pages) survives.
38. **For a hung action, the trace shows the action's `before` event with no `after`.** Render the DOM snapshot at that instant to see what blocked it.
39. **"Flaky" or "CI infra" is not an accepted root cause for a recurring failure.** Check history and backend changes. Refute a candidate cause by its other necessary consequences: a cause that explains the symptom but whose side effects are absent is wrong. When two causes changed at the same time, find a time window that straddles only one of them.
40. **CI worsening across reruns of one commit is a clock, not a commit:** an expiring credential or a moving external dependency, not the diff.
41. **When tests block a release, the first PR out is a `fixme` skip on the release branch.** The real fix comes after, and targets the release branch too.

## 6. Process scope

42. **The QA team writes e2e, not backend unit tests.** An async end state with genuinely no UI surface goes to developers as a coverage note, not into a Playwright fake.
43. **Bug-hunt reports list application bugs only.** Test-automation defects are fixed and noted in the changelog.
44. **An independent worker verifies that every designed interaction and branch was exercised before a feature closes.** The author's own checklist is not that check.
45. **Dead code found while fixing is removed in the same PR,** not deferred.
46. **A candidate bug is not real until it is reproduced live and checked against written intent.** Code shows how the product does behave; the intent source shows how it should. On conflict, flag the gap rather than silently following either.
