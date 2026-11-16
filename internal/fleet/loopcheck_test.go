package fleet

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// checkFixture is a loop fixture whose worktree exists, plus the
// single command-gated step "a" and its round-1 output.
func checkFixture(t *testing.T, run string, timeout int) (*loopFixture, Step, Condition) {
	t.Helper()
	f := newLoopFixture(t, "checked", "checked.toml", "alpha")
	f.tick(t)
	f.write(t, "a/round-1.json", `{"result":"x"}`)
	c := Condition{Kind: "command", Run: run, TimeoutSeconds: timeout}
	s := Step{Name: "a", Kind: "agent", Output: "a/round-{round}.json", Done: []Condition{c}}
	return f, s, c
}

func countingRun(t *testing.T, exit int) (run, count string) {
	t.Helper()
	count = filepath.Join(t.TempDir(), "count")
	return fmt.Sprintf("echo run >> '%s'; echo out-$((1+1)); exit %d", count, exit), count
}

func TestCommandConditionRunsOncePerFingerprint(t *testing.T) {
	run, count := countingRun(t, 1)
	f, s, c := checkFixture(t, run, 30)

	for i := 0; i < 3; i++ {
		passed, out, fp, err := evalCommand(f.root, f.slug, s, 1, c)
		if err != nil {
			t.Fatalf("evalCommand: %v", err)
		}
		if passed || !strings.Contains(out, "out-2") || len(fp) < 12 {
			t.Fatalf("evalCommand = (%v, %q, %q), want failed with output out-2 and a fingerprint", passed, out, fp)
		}
	}
	if n := fileLines(t, count); n != 1 {
		t.Errorf("command ran %d times for one fingerprint, want 1", n)
	}
	if m := f.stateMarkers("check-a-r1-"); len(m) != 1 || !strings.HasSuffix(m[0], ".json") {
		t.Errorf("cached results = %v, want one state/check-a-r1-<fp12>.json", m)
	}

	f.write(t, "a/round-1.json", `{"result":"y"}`)
	if _, _, _, err := evalCommand(f.root, f.slug, s, 1, c); err != nil {
		t.Fatal(err)
	}
	if n := fileLines(t, count); n != 2 {
		t.Errorf("after the output changed the command ran %d times in total, want 2", n)
	}
}

func TestCommandConditionNewHeadReruns(t *testing.T) {
	run, count := countingRun(t, 1)
	f, s, c := checkFixture(t, run, 30)

	if _, _, fp1, err := evalCommand(f.root, f.slug, s, 1, c); err != nil {
		t.Fatal(err)
	} else {
		wt := filepath.Join(f.root, ".worktrees", f.slug)
		if out, err := exec.Command("git", "-C", wt, "commit", "-q", "--allow-empty", "-m", "fix").CombinedOutput(); err != nil {
			t.Fatalf("commit in worktree: %v: %s", err, out)
		}
		_, _, fp2, err := evalCommand(f.root, f.slug, s, 1, c)
		if err != nil {
			t.Fatal(err)
		}
		if fp1 == fp2 {
			t.Errorf("fingerprint unchanged across a new worktree HEAD: %s", fp1)
		}
	}
	if n := fileLines(t, count); n != 2 {
		t.Errorf("command ran %d times across two HEADs, want 2", n)
	}
}

func TestCommandConditionTimeout(t *testing.T) {
	f, s, c := checkFixture(t, "sleep 5", 1)

	start := time.Now()
	passed, out, _, err := evalCommand(f.root, f.slug, s, 1, c)
	if err != nil {
		t.Fatalf("evalCommand: %v", err)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Errorf("a hung command held the tick for %v, want about its 1s timeout", took)
	}
	if passed {
		t.Errorf("timed-out command passed")
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "timed out after 1s") {
		t.Errorf("output = %q, want it to end with %q", out, "timed out after 1s")
	}
}

func TestCommandConditionPassSticksForItsRound(t *testing.T) {
	run, count := countingRun(t, 0)
	f, s, c := checkFixture(t, run, 30)

	if passed, _, _, err := evalCommand(f.root, f.slug, s, 1, c); err != nil || !passed {
		t.Fatalf("evalCommand = %v, %v; want passed", passed, err)
	}
	wt := filepath.Join(f.root, ".worktrees", f.slug)
	if out, err := exec.Command("git", "-C", wt, "commit", "-q", "--allow-empty", "-m", "later").CombinedOutput(); err != nil {
		t.Fatalf("commit in worktree: %v: %s", err, out)
	}
	if passed, _, _, err := evalCommand(f.root, f.slug, s, 1, c); err != nil || !passed {
		t.Fatalf("after a new HEAD evalCommand = %v, %v; want still passed", passed, err)
	}
	if n := fileLines(t, count); n != 1 {
		t.Errorf("a passed round re-ran its check %d times in total, want 1", n)
	}
}

const cappedCheckLoop = `name = "checked"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
session = "a"
output = "a/round-{round}.json"
shape = "json"
required = ["result"]
max_rounds = 1

[[steps.done]]
kind = "command"
run = "test -e \"$PASS_FILE\" || { echo nope; exit 1; }"
timeout = "30s"

[[steps.next]]
to = "done"
`

func TestCommandFailureDoesNotCountTowardMaxRounds(t *testing.T) {
	f := newLoopFixture(t, "checked", "checked.toml", "alpha")
	if err := os.WriteFile(filepath.Join(f.root, "bootstrap", "loops", "checked.toml"), []byte(cappedCheckLoop), 0o644); err != nil {
		t.Fatal(err)
	}
	pass := filepath.Join(t.TempDir(), "pass")
	t.Setenv("PASS_FILE", pass)

	f.tick(t)
	for i := 1; i <= 3; i++ {
		f.write(t, "a/round-1.json", `{"result":"`+strings.Repeat("x", i)+`"}`)
		st := f.tick(t)
		if st.Terminal != "" || st.Step != "a" || st.Blocked == "" {
			t.Fatalf("failing check %d: state = %+v, want blocked on a, not terminal", i, st)
		}
	}
	if m := f.stateMarkers("escalated-"); len(m) != 0 {
		t.Errorf("failing checks escalated (%v); they must not count toward max_rounds", m)
	}
	if n := countContaining(inboxBodies(t, f.inbox), "check failed"); n != 3 {
		t.Errorf("coordinator got %d check-failed notices for three distinct outputs, want 3", n)
	}

	if err := os.WriteFile(pass, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.write(t, "a/round-1.json", `{"result":"final"}`)
	if st := f.tick(t); st.Terminal != "done" {
		t.Errorf("passing check: state = %+v, want Terminal done", st)
	}
}

const devOverrideLoop = `name = "dev"

[[steps]]
name = "engineer"
kind = "agent"
role = "engineer"
session = "engineer"
phase = "engineer-initial"
output = "responses/engineer-round-{round}.json"
shape = "engineer-response"

[[steps.next]]
to = "done"
`

func TestDeriveSnapshotUsesProjectDevOverride(t *testing.T) {
	dirs := newTestDirs(t)
	p := filepath.Join(dirs.project, "bootstrap", "loops", "dev.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(devOverrideLoop), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDevStep(t, dirs.project, "demo", devStep{who: "eng", round: 1})

	snap, err := DeriveSnapshot(dirs.project, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Phase != PhaseDone {
		t.Errorf("snapshot phase = %q, want %q from the project's dev.toml (engineer then done)", snap.Phase, PhaseDone)
	}
}
