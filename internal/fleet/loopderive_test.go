package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task"
)

func mustLoopDef(t *testing.T, src string) LoopDef {
	t.Helper()
	def, err := ParseLoopDef("test.toml", []byte(src))
	if err != nil {
		t.Fatalf("ParseLoopDef: %v", err)
	}
	return def
}

func mustDevDef(t *testing.T) LoopDef {
	t.Helper()
	def, err := LoadLoop(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("LoadLoop dev: %v", err)
	}
	return def
}

// outs builds step outputs from a compact spec: each entry is a
// verdict ("" for a non-verdict output), rounds numbered from 1.
func outs(byStep map[string][]string) map[string][]StepOutput {
	m := map[string][]StepOutput{}
	for step, verdicts := range byStep {
		for i, v := range verdicts {
			m[step] = append(m[step], StepOutput{Round: i + 1, Verdict: v})
		}
	}
	return m
}

func TestDeriveLoopStateTable(t *testing.T) {
	const (
		ok = task.VerdictApprove
		rc = task.VerdictRequestChanges
	)
	def := mustDevDef(t)
	cases := []struct {
		name string
		in   map[string][]string
		want LoopState
	}{
		{"no engineer response", nil,
			LoopState{Step: "engineer", Phase: "engineer-initial"}},
		{"engineer shipped, A silent", map[string][]string{"engineer": {""}},
			LoopState{Step: "review-A", Phase: "review-A"}},
		{"A requests changes, engineer silent", map[string][]string{"engineer": {""}, "review-A": {rc}},
			LoopState{Step: "engineer", Phase: "engineer-revise-A", Round: 1, ReturnFrom: "review-A", SenderRound: 1, LastVerdict: rc}},
		{"engineer answered A", map[string][]string{"engineer": {"", ""}, "review-A": {rc}},
			LoopState{Step: "review-A", Phase: "review-A", Round: 1, LastVerdict: rc}},
		{"A requests changes at round 3", map[string][]string{"engineer": {"", "", ""}, "review-A": {rc, rc, rc}},
			LoopState{Step: "review-A", Phase: "escalated", Terminal: "escalated", Round: 3, LastVerdict: rc}},
		{"A approved, B silent", map[string][]string{"engineer": {""}, "review-A": {ok}},
			LoopState{Step: "review-B", Phase: "review-B", LastVerdict: ok}},
		{"B requests changes, engineer silent", map[string][]string{"engineer": {""}, "review-A": {ok}, "review-B": {rc}},
			LoopState{Step: "engineer", Phase: "engineer-revise-B", Round: 1, ReturnFrom: "review-B", SenderRound: 1, LastVerdict: rc}},
		{"engineer answered B", map[string][]string{"engineer": {"", ""}, "review-A": {ok}, "review-B": {rc}},
			LoopState{Step: "review-B", Phase: "review-B", Round: 1, LastVerdict: rc}},
		{"B requests changes at round 3", map[string][]string{"engineer": {"", "", ""}, "review-A": {ok}, "review-B": {rc, rc, rc}},
			LoopState{Step: "review-B", Phase: "escalated", Terminal: "escalated", Round: 3, LastVerdict: rc}},
		{"B approved", map[string][]string{"engineer": {""}, "review-A": {ok}, "review-B": {ok}},
			LoopState{Step: "review-B", Phase: "done", Terminal: "done", Round: 1, LastVerdict: ok}},
		// Upstream counts engineer files: engCount >= expected. An
		// early second response already answers A's request.
		{"early engineer file answers A", map[string][]string{"engineer": {"", ""}, "review-A": {rc}},
			LoopState{Step: "review-A", Phase: "review-A", Round: 1, LastVerdict: rc}},
		{"early engineer file answers B", map[string][]string{"engineer": {"", ""}, "review-A": {ok}, "review-B": {rc}},
			LoopState{Step: "review-B", Phase: "review-B", Round: 1, LastVerdict: rc}},
		{"spare engineer file waits for A", map[string][]string{"engineer": {"", "", ""}},
			LoopState{Step: "review-A", Phase: "review-A"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deriveLoopState(def, outs(c.in), nil, nil)
			got.Rounds = nil
			c.want.Loop = "dev"
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got: %+v\nwant: %+v", got, c.want)
			}
		})
	}
}

// The returned-to step goes back to its sender, not to its own next.
func TestDeriveLoopStateReturnGoesBackToSender(t *testing.T) {
	def := mustLoopDef(t, `
name = "ret"

[[steps]]
name = "author"
kind = "agent"
role = "author"
output = "author/round-{round}.json"
shape = "json"

[[steps]]
name = "middle"
kind = "agent"
role = "middle"
output = "middle/round-{round}.json"
shape = "json"

[[steps]]
name = "review"
kind = "agent"
role = "reviewer"
output = "review/round-{round}.json"
shape = "verdict"
max_rounds = 3

[[steps.next]]
on = "request_changes"
to = "author"
return = true
`)
	in := outs(map[string][]string{"author": {""}, "middle": {""}, "review": {task.VerdictRequestChanges}})
	st := deriveLoopState(def, in, nil, nil)
	if st.Step != "author" || st.Phase != "author-revise-review" || st.ReturnFrom != "review" || st.SenderRound != 1 {
		t.Fatalf("after request_changes: %+v, want author serving a return from review round 1", st)
	}
	in["author"] = append(in["author"], StepOutput{Round: 2})
	st = deriveLoopState(def, in, nil, nil)
	if st.Step != "review" || st.Phase != "review" || st.ReturnFrom != "" {
		t.Fatalf("after author round 2: %+v, want back at review (not middle)", st)
	}
	if st.Rounds["middle"] != 1 || st.Rounds["author"] != 2 {
		t.Errorf("rounds = %v, want middle 1, author 2", st.Rounds)
	}
	in["review"] = append(in["review"], StepOutput{Round: 2, Verdict: task.VerdictApprove})
	st = deriveLoopState(def, in, nil, nil)
	if st.Terminal != "done" {
		t.Errorf("review approved on the last step: %+v, want Terminal done", st)
	}
}

func TestDeriveLoopStateActionVisits(t *testing.T) {
	def := mustLoopDef(t, `
name = "act"

[[steps]]
name = "author"
kind = "agent"
role = "author"
output = "author/round-{round}.json"
shape = "json"

[[steps]]
name = "open-pr"
kind = "action"
action = "notify-coordinator"

[[steps]]
name = "review"
kind = "agent"
role = "reviewer"
output = "review/round-{round}.json"
shape = "verdict"

[[steps.next]]
on = "request_changes"
to = "author"
`)
	in := outs(map[string][]string{"author": {""}})
	st := deriveLoopState(def, in, map[string]int{}, nil)
	if st.Step != "open-pr" || st.Round != 0 {
		t.Fatalf("no visit done: %+v, want Step open-pr round 0", st)
	}
	st = deriveLoopState(def, in, map[string]int{"open-pr": 1}, nil)
	if st.Step != "review" {
		t.Fatalf("one visit done: %+v, want Step review", st)
	}
	in = outs(map[string][]string{"author": {"", ""}, "review": {task.VerdictRequestChanges}})
	st = deriveLoopState(def, in, map[string]int{"open-pr": 1}, nil)
	if st.Step != "open-pr" || st.Round != 1 {
		t.Fatalf("author revised, second visit pending: %+v, want Step open-pr round 1", st)
	}
	st = deriveLoopState(def, in, map[string]int{"open-pr": 2}, nil)
	if st.Step != "review" || st.Rounds["open-pr"] != 2 {
		t.Fatalf("second visit done: %+v, want Step review with open-pr 2", st)
	}
}

func TestDeriveLoopStateBlockedByGate(t *testing.T) {
	def := mustLoopDef(t, `
name = "gated"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
output = "a/round-{round}.json"
shape = "json"

[[steps]]
name = "b"
kind = "agent"
role = "beta"
output = "b/round-{round}.json"
shape = "json"
`)
	in := outs(map[string][]string{"a": {""}})
	var asked []string
	st := deriveLoopState(def, in, nil, func(step string, round int) bool {
		asked = append(asked, step)
		return false
	})
	if st.Step != "a" || st.Blocked == "" || st.Round != 1 {
		t.Fatalf("gate fails: %+v, want Step a, round 1, Blocked set", st)
	}
	if len(asked) != 1 || asked[0] != "a" {
		t.Errorf("gate asked for %v, want [a]", asked)
	}
	st = deriveLoopState(def, in, nil, func(string, int) bool { return true })
	if st.Step != "b" || st.Blocked != "" {
		t.Errorf("gate holds: %+v, want Step b, not blocked", st)
	}
}

// Review Focus 1: a loop file edited mid-loop. Outputs and action
// markers of steps that no longer exist are ignored, never a panic.
func TestDeriveLoopStateIgnoresUnknownStepOutputs(t *testing.T) {
	root, slug := t.TempDir(), "demo"
	dir := task.RoleTaskDir(root, slug)
	writeRaw(t, filepath.Join(dir, "a", "round-1.json"), `{"result":"x"}`)
	writeRaw(t, filepath.Join(dir, "b", "round-1.json"), `{"result":"y"}`)
	writeRaw(t, filepath.Join(dir, "state", "action-old-v1"), "")

	renamed := mustLoopDef(t, `
name = "trio"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
output = "a/round-{round}.json"
shape = "json"
required = ["result"]

[[steps]]
name = "b2"
kind = "agent"
role = "beta"
output = "b2/round-{round}.json"
shape = "json"
required = ["result"]

[[steps]]
name = "c"
kind = "action"
action = "notify-coordinator"
`)
	st, err := DeriveLoopState(root, slug, renamed, nil)
	if err != nil {
		t.Fatalf("DeriveLoopState: %v", err)
	}
	if st.Step != "b2" || st.Round != 0 || st.Terminal != "" {
		t.Errorf("state = %+v, want Step b2 round 0 (b's file belongs to no step)", st)
	}
	if _, ok := st.Rounds["b"]; ok {
		t.Errorf("rounds = %v, counted the removed step b", st.Rounds)
	}

	got := deriveLoopState(renamed,
		map[string][]StepOutput{"a": {{Round: 1}}, "gone": {{Round: 1, Verdict: "approve"}}},
		map[string]int{"old": 3}, nil)
	if got.Step != "b2" {
		t.Errorf("pure derive with unknown keys: %+v, want Step b2", got)
	}
}

// Review Focus 4: valid JSON that lacks a declared required field is
// not counted, and the bounce path quarantines it after the wake.
func TestCustomShapeMissingFieldNotCounted(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	slug := "demo"
	if err := task.EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	def := mustLoopDef(t, `
name = "custom"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
session = "a"
output = "a/round-{round}.json"
shape = "json"
required = ["result"]

[[steps]]
name = "b"
kind = "agent"
role = "beta"
output = "b/round-{round}.json"
shape = "json"
required = ["result"]
`)
	path := filepath.Join(task.RoleTaskDir(root, slug), "a", "round-1.json")
	for _, body := range []string{`{"result": null}`, `{"other": 1}`, `["result"]`} {
		writeRaw(t, path, body)
		st, err := DeriveLoopState(root, slug, def, nil)
		if err != nil {
			t.Fatalf("%s: DeriveLoopState: %v", body, err)
		}
		if st.Step != "a" || st.Round != 0 {
			t.Errorf("%s: state = %+v, want Step a round 0 (output not counted)", body, st)
		}
	}
	writeRaw(t, path, `{"result": null}`)

	files, err := task.ListRoleFiles(root, slug, loopStepFiles(def)...)
	if err != nil {
		t.Fatalf("ListRoleFiles: %v", err)
	}
	var found *task.RoleFile
	for i := range files {
		if files[i].Path == path {
			found = &files[i]
		}
	}
	if found == nil {
		t.Fatalf("ListRoleFiles = %+v, missing %s", files, path)
	}
	verr := task.ValidateRoleFile(root, slug, *found)
	if !errors.Is(verr, task.ErrInvalidRoleFile) || !strings.Contains(verr.Error(), `"result"`) {
		t.Fatalf("ValidateRoleFile = %v, want an invalid-role-file error naming \"result\"", verr)
	}

	outcome := wakePending
	calls := stubWakeRole(t, &outcome)
	if err := bounceInvalidRoleFiles(root, slug, loopStepFiles(def)...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file moved before the wake was delivered: %v", err)
	}
	if len(*calls) != 1 || !strings.HasSuffix((*calls)[0].session, "/demo/a") {
		t.Fatalf("wakes = %+v, want one to step a's session", *calls)
	}
	if msg := (*calls)[0].msg; !strings.Contains(msg, `"result"`) {
		t.Errorf("wake msg does not name the required key: %s", msg)
	}
	outcome = wakeDelivered
	if err := bounceInvalidRoleFiles(root, slug, loopStepFiles(def)...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + task.InvalidSuffix); err != nil {
		t.Errorf("quarantined copy missing after delivery: %v", err)
	}

	writeRaw(t, path, `{"result": "x"}`)
	st, err := DeriveLoopState(root, slug, def, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != "b" {
		t.Errorf("valid output: state = %+v, want Step b", st)
	}
}

// A skipped invalid file must not shift the round the gate checks: the
// gate and SenderRound use the round number of the output on disk.
func TestDeriveLoopStateGateUsesFileRound(t *testing.T) {
	root, slug := t.TempDir(), "demo"
	dir := task.RoleTaskDir(root, slug)
	def := mustLoopDef(t, `
name = "filed"

[[steps]]
name = "a"
kind = "agent"
role = "alpha"
output = "a/round-{round}.json"
shape = "json"
required = ["result"]

[[steps.done]]
kind = "file"
path = "a/check-{round}.txt"

[[steps]]
name = "b"
kind = "agent"
role = "beta"
output = "b/round-{round}.json"
shape = "verdict"

[[steps.next]]
on = "request_changes"
to = "a"
return = true
`)
	writeRaw(t, filepath.Join(dir, "a", "round-1.json"), `{"result": null}`)
	writeRaw(t, filepath.Join(dir, "a", "round-2.json"), `{"result": "x"}`)
	writeRaw(t, filepath.Join(dir, "a", "check-2.txt"), "")
	writeRaw(t, filepath.Join(dir, "b", "round-1.json"), `{"author":"b","verdict":"approve","summary":"s"}`)
	writeRaw(t, filepath.Join(dir, "b", "round-2.json"), `{"author":"b","verdict":"request_changes","summary":"s"}`)
	st, err := DeriveLoopState(root, slug, def, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Blocked != "" {
		t.Fatalf("state = %+v, want a's round-2 output gated on check-2.txt, not check-1.txt", st)
	}
	// b round-1 is "approve" with no transition: the loop ends there.
	if st.Terminal != "done" {
		t.Errorf("state = %+v, want done", st)
	}

	writeRaw(t, filepath.Join(dir, "b", "round-1.json"), `not json`)
	st, err = DeriveLoopState(root, slug, def, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != "a" || st.ReturnFrom != "b" || st.SenderRound != 2 {
		t.Errorf("state = %+v, want a serving a return from b round 2 (the file's round)", st)
	}
}

// TestDeriveLoopStateMaxRoundsCountsApprovedRounds pins max_rounds as
// documented ("once the step has written this many outputs"): review
// approves rounds 1 and 2, a later completeness step sends the work
// back each time, and review's first request_changes, in round 3,
// escalates. An approval that moves forward never escalates.
func TestDeriveLoopStateMaxRoundsCountsApprovedRounds(t *testing.T) {
	def := mustLoopDef(t, `
name = "qa"

[[steps]]
name = "author"
kind = "agent"
role = "author"
output = "author/round-{round}.json"
shape = "json"

[[steps]]
name = "review"
kind = "agent"
role = "reviewer"
output = "review/round-{round}.json"
shape = "verdict"
max_rounds = 3

[[steps.next]]
on = "request_changes"
to = "author"
return = true

[[steps]]
name = "completeness"
kind = "agent"
role = "reviewer"
output = "completeness/round-{round}.json"
shape = "verdict"

[[steps.next]]
on = "request_changes"
to = "author"
`)
	A, RC := task.VerdictApprove, task.VerdictRequestChanges
	in := outs(map[string][]string{
		"author":       {"", "", ""},
		"review":       {A, A, A},
		"completeness": {RC, RC},
	})
	if st := deriveLoopState(def, in, nil, nil); st.Terminal != "" || st.Step != "completeness" {
		t.Fatalf("review approving round 3: %+v, want completeness, not escalated", st)
	}
	in["review"][2].Verdict = RC
	st := deriveLoopState(def, in, nil, nil)
	if st.Terminal != "escalated" || st.Step != "review" || st.Round != 3 {
		t.Fatalf("review's first request_changes in round 3: %+v, want escalated at review round 3", st)
	}
}
