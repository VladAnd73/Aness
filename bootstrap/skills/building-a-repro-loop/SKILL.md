---
name: building-a-repro-loop
description: Use when a failure cannot be triggered on demand - a flaky or intermittent test, a failure seen only in CI or only on another machine, an e2e or browser test that fails sometimes, a bug report with no repro steps, "cannot reproduce", a timing or race symptom, or a performance regression - and also when systematic debugging stalls at "reproduce consistently".
---

# building-a-repro-loop

## Overview

Before you form any theory about a hard bug, build **one command that
goes red on this bug**: fast, the same verdict every run, and runnable
without a human. With that command, bisecting, testing hypotheses and
proving the fix are mechanical. Without it, reading code produces
guesses.

**Core rule:** no hypothesis before the loop exists and you have seen
it go red.

This skill is the reproduce step done properly. If
`superpowers:systematic-debugging` is also loaded, this skill replaces
its "Reproduce consistently" step; take its later phases once your loop
is red and minimal. For a bug that already fails on every run with a
clear error, you do not need this skill.

## Redact

You will paste commands and output into reports. Replace every secret
with `<REDACTED>` first. Build loops that read credentials from env
vars, so the secret never shows up in what you paste. Captured traffic
carries auth headers: quote only the lines that carry the signal.

## 1. Build the loop

Spend most of your effort here. Try these, roughly in order:

1. **Failing test** at whatever level reaches the bug: unit,
   integration, e2e.
2. **HTTP script** (`curl`) against a running dev server.
3. **CLI run** on a fixture input, diffing output against a known-good
   copy.
4. **Headless browser script** (Playwright) that drives the UI and
   asserts on the page, console or network.
5. **Replay a capture.** Save a real request, payload or event log to
   disk and push it through the code path alone.
6. **Throwaway harness.** Start the smallest slice of the system (one
   service, fake dependencies) that hits the bug with one call.
7. **Fuzz loop.** For "sometimes wrong output", run 1000 random inputs
   and look for the failure.
8. **Bisect harness.** If the bug appeared between two known states
   (commits, versions, datasets), script "set up state X, check" and
   hand it to `git bisect run`.
9. **Differential loop.** Same input through old and new versions (or
   two configs); diff the outputs.
10. **Human-driven script.** Last resort, and only when an operator is
    present in the session: copy `scripts/hitl-loop.template.sh`, edit
    the steps, and start it where the operator can type, since the Bash
    tool has no terminal:
    `tmux new-window -d -n repro "bash repro.sh | tee repro.out"`.
    Tell the operator the window name, then read `repro.out`. A
    headless worker never uses this; see "When you cannot build a
    loop".

### Tighten it

- **Faster**: cache setup, skip unrelated init, narrow the test scope.
- **Sharper**: assert the exact symptom, not "did not crash".
- **Deterministic**: pin the clock, seed the random generator, isolate
  the filesystem, cut the network.

A 30-second flaky loop is barely better than none. A 2-second
deterministic one is what you want.

### Flaky and intermittent bugs

The goal is a **higher failure rate**, not a clean single repro. Run
the trigger 100 times, run copies in parallel, add load, narrow timing
windows, inject sleeps. A bug that fails half the time is debuggable;
one that fails 1% of the time is not. Keep raising the rate. Report it
as "red in N of M runs".

```bash
# Run a test 200 times and count failures; the rate is your baseline.
fails=0
for i in $(seq 200); do
  go test ./pkg/... -run TestFlaky -count=1 >/dev/null 2>&1 || fails=$((fails+1))
done
echo "red in $fails of 200"
```

### Done when

You can name one command you have **already run**, with its output
pasted (redacted), that is:

- [ ] **Red on this bug**: it drives the real code path and checks the
      reported symptom, so it can go green once fixed.
- [ ] **Deterministic**: same verdict every run, or a pinned high
      failure rate for a flaky bug.
- [ ] **Fast**: seconds, not minutes.
- [ ] **Unattended**: you can run it with no human.

If you catch yourself reading code to build a theory before this
command exists, stop. That is the failure this skill exists to prevent.

## 2. Reproduce and shrink

Run the loop and watch it go red. Check:

- [ ] It fails the way the **report** described, not a nearby failure.
      Wrong bug means wrong fix.
- [ ] It fails across runs (or at the pinned rate).
- [ ] You captured the exact symptom (message, wrong value, timing), so
      you can show the fix removes it.

Then shrink it. Cut inputs, callers, config, data and steps **one at a
time**, re-running after each cut. Keep only what the failure needs.
Done when removing any remaining piece turns the loop green. The small
repro narrows the suspects and becomes the regression test.

## 3. After the loop

- **Hypotheses**: write 3 to 5, ranked, before testing any. Each states
  a prediction: "If X is the cause, then changing Y makes the bug go
  away." No prediction, no hypothesis.
- **Do not wait for approval.** A headless worker tests its ranked list
  in order and writes it, with the outcome of each, into the report or
  the DONE tell. In an interactive session, show the list once (the
  operator may know one is already ruled out), then carry on.
- **Probes**: one variable at a time. Tag every debug log with one
  prefix (for example `[DEBUG-a4f2]`) so cleanup is a single grep.
  For a performance regression, measure a baseline first; logs rarely
  help there.
- **Regression test**: turn the shrunk repro into a failing test at a
  level that reproduces the real call pattern, watch it fail, fix,
  watch it pass, then re-run the original loop. If no such level
  exists, say so in the report: that is a finding about the code.
- **Cleanup**: original loop is green, the `[DEBUG-` prefix greps to
  nothing, throwaway harnesses are deleted, and the commit message
  names the hypothesis that turned out right.

## When you cannot build a loop

Stop and say so. Do not move on to hypotheses.

- **Headless worker**: report BLOCKED to the coordinator. List every
  loop you tried and why each failed, and ask for one of: access to
  the environment that reproduces it, a redacted capture (log dump, HAR
  file, core dump, CI artifact), or permission to add temporary
  instrumentation where it fails.
- **Interactive session**: ask the operator for the same things. If a
  human must click to trigger it, use the human-driven script above.

## Common mistakes

| Mistake | Fix |
| --- | --- |
| "I can see the bug in the code, no need for a loop" | Then the loop takes a minute to write. Write it; it is also your regression test. |
| Loop asserts "exit code 0" | Assert the reported symptom. |
| Calling a 1-in-50 flake "not reproducible" | Run it 200 times and raise the rate. |
| Fixing a nearby failure the loop happened to hit | Check the symptom matches the report before going on. |
| Waiting for the operator to approve hypotheses in a headless run | Test them in order and report them. |
