package watch

import (
	"fmt"
	"strings"
	"time"
)

// now is the package clock, overridable in tests.
var now = func() time.Time { return time.Now() }

// failureCooldown is how long an unchanged ill-health alert stays suppressed
// before it re-surfaces once as a heartbeat, so a genuinely stuck condition is
// never silently swallowed.
const failureCooldown = 4 * time.Hour

type Report struct {
	Alerts  int
	Skipped int
}

const runbook = `Runbook (per approved spec docs/todo/pr-e2e-watcher.md in the spore repo):
1. Mint a worker task: fetch origin, check out the PR branch, pull, run the FAILED specs locally (use the project's running-*-e2e / investigating-*-e2e-failures skills).
2. Worker tells you the local result.
3. Local PASS -> retry CI once: spore-with-secrets gh run rerun <run-id from the link> --failed. Watch the rerun; green = done silently, red = escalate to operator.
4. Local FAIL -> escalate to operator in your terminal with the worker's findings. Do not retry.
Max ONE CI retry per commit. Escalate instead of looping.`

// prAlert holds a rollup tell for one PR.
type prAlert struct {
	keys []string
	msg  string
}

func Run(projectRoot, project string, dryRun bool, tell func(slug, msg string) error) (Report, error) {
	var rep Report
	cfg, err := LoadConfig(project)
	if err != nil || !cfg.Enabled {
		return rep, err
	}
	st, err := LoadState(project)
	if err != nil {
		return rep, err
	}
	prs, err := OpenPRs(projectRoot)
	if err != nil {
		return rep, noteFailure(st, dryRun, tell, err)
	}
	live := map[string]bool{}
	var alerts []prAlert
	// A single PR whose checks query errors is skipped so it cannot blind the
	// watcher to the PRs after it. Only a total wipeout (every evaluated PR
	// errors) is treated as ill-health.
	var evaluated, perPRErrors int
	var lastPRErr error
	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		evaluated++
		// Keys live as long as their PR is open at the same head SHA. A CI
		// rerun puts a failed check back to pending, and pruning on "failing
		// right now" made its next failure on the same commit alert again.
		// A new push changes the SHA, so its failures still alert.
		prefix := fmt.Sprintf("%d:%s:", pr.Number, pr.HeadSHA)
		for k := range st.Seen {
			if strings.HasPrefix(k, prefix) {
				live[k] = true
			}
		}
		checks, err := FailingChecks(projectRoot, pr.Number)
		if err != nil {
			perPRErrors++
			lastPRErr = err
			continue
		}
		var newKeys []string
		var lines []string
		for _, c := range checks {
			if !nameMatches(c.Name, cfg.Checks) {
				continue
			}
			k := Key(pr.Number, pr.HeadSHA, c.Name)
			live[k] = true
			if st.SeenKey(k) {
				rep.Skipped++
				continue
			}
			newKeys = append(newKeys, k)
			lines = append(lines, fmt.Sprintf("  %s\n  run: %s", c.Name, c.Link))
		}
		if len(newKeys) == 0 {
			continue
		}
		msg := fmt.Sprintf(
			"pr-watch: PR #%d (%s) has %d failing e2e check(s)\n%s\npr: %s\n\n%s",
			pr.Number, pr.Branch, len(newKeys),
			strings.Join(lines, "\n"),
			pr.URL, runbook)
		alerts = append(alerts, prAlert{newKeys, msg})
	}
	if evaluated > 0 && perPRErrors == evaluated {
		return rep, noteFailure(st, dryRun, tell,
			fmt.Errorf("all %d open PR(s) failed their checks query: %w", evaluated, lastPRErr))
	}
	if dryRun {
		rep.Alerts = len(alerts)
		return rep, nil
	}
	for _, a := range alerts {
		if err := tell("coordinator", a.msg); err != nil {
			_ = st.Save()
			return rep, err
		}
		for _, k := range a.keys {
			st.MarkKey(k)
		}
		if err := st.Save(); err != nil {
			return rep, err
		}
		rep.Alerts++
	}
	st.Prune(live)
	if perPRErrors > 0 {
		// Partial-error poll: healthy PRs were evaluated and alerted, so the poll
		// is not aborted, but a gh failure still occurred. Advance the ill-health
		// record (do not reset it) so repeated partial outages still escalate.
		return rep, recordFailure(st, tell,
			fmt.Errorf("%d of %d open PR(s) failed their checks query: %w", perPRErrors, evaluated, lastPRErr))
	}
	// A fully clean poll clears the failure record so the next failure alerts
	// immediately rather than waiting out the cooldown.
	st.Failures = 0
	st.NotifiedSig = ""
	st.NotifiedAt = ""
	return rep, st.Save()
}

func nameMatches(name string, patterns []string) bool {
	l := strings.ToLower(name)
	for _, p := range patterns {
		if strings.Contains(l, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// failureSig is the dedup key for an ill-health alert. cause.Error() already
// embeds the gh subcommand (PR number, args) and the exit status, so a trimmed
// copy is a stable "same failure" signature. Its exact shape is pinned by
// TestFailureSigShape rather than assumed.
func failureSig(cause error) string {
	return strings.TrimSpace(cause.Error())
}

// cooldownElapsed reports whether failureCooldown has passed since notifiedAt.
// An empty or unparseable timestamp means "notify" so a real alert is never
// swallowed by a bad record.
func cooldownElapsed(notifiedAt string) bool {
	if notifiedAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, notifiedAt)
	if err != nil {
		return true
	}
	return now().Sub(t) >= failureCooldown
}

// recordFailure advances the consecutive-failure counter and alerts the
// coordinator only when the failure is sustained (>= 3 consecutive) AND the
// signature is new or the cooldown has elapsed. It persists the updated record
// and returns only a persistence error, so callers decide whether the poll as
// a whole failed. Used by both the total-failure path (via noteFailure) and
// the partial-error path, so an intermittent outage still escalates.
func recordFailure(st *State, tell func(string, string) error, cause error) error {
	st.Failures++
	sig := failureSig(cause)
	notify := st.Failures >= 3 && (sig != st.NotifiedSig || cooldownElapsed(st.NotifiedAt))
	if notify {
		st.NotifiedSig = sig
		st.NotifiedAt = now().UTC().Format(time.RFC3339)
	}
	// Save the incremented counter (and notify record) before telling: if Save
	// fails the tell is suppressed (silent gap) rather than firing again next
	// round (duplicate).
	if err := st.Save(); err != nil {
		return err
	}
	if notify {
		_ = tell("coordinator",
			fmt.Sprintf("pr-watch: unhealthy, %d consecutive polling failures. Last error: %v", st.Failures, cause))
	}
	return nil
}

// noteFailure is the total-failure path (top-level pr list error, or every PR
// erroring). In dry-run it is a pure probe: it surfaces the failure via the
// returned error but neither tells nor mutates state. Otherwise it records the
// failure and returns cause so the poll surfaces the error to its caller.
func noteFailure(st *State, dryRun bool, tell func(string, string) error, cause error) error {
	if dryRun {
		return cause
	}
	if err := recordFailure(st, tell, cause); err != nil {
		return err
	}
	return cause
}
