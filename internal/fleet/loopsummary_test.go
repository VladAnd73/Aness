package fleet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDriveLoopCustomSummaryNamesItsOwnSteps: summary = true on a loop
// whose steps are not the dev loop's writes summary.md from that
// loop's own step outputs.
func TestDriveLoopCustomSummaryNamesItsOwnSteps(t *testing.T) {
	src := strings.Replace(mustReadFile(t, filepath.Join("testdata", "loops", "trio.toml")), `name = "trio"`, "name = \"qa\"\nsummary = true", 1)
	f := newSrcLoopFixture(t, "qa", src, "alpha", "beta")
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", "qa")
	f.write(t, "a/round-1.json", `{"result":"specs written"}`)
	f.write(t, "b/round-1.json", `{"result":"coverage complete"}`)
	var st LoopState
	for i := 0; i < 4 && st.Terminal == ""; i++ {
		st = f.tick(t)
	}
	if st.Terminal != "done" {
		t.Fatalf("loop did not finish: %+v", st)
	}
	body := mustReadFile(t, f.path("summary.md"))
	for _, want := range []string{"## a", "specs written", "## b", "coverage complete"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary.md lacks %q:\n%s", want, body)
		}
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// doneQALoop drives the trio loop, renamed qa with summary = true, to
// done with one round from each agent step.
func doneQALoop(t *testing.T, aOut, bOut string) *loopFixture {
	t.Helper()
	src := strings.Replace(mustReadFile(t, filepath.Join("testdata", "loops", "trio.toml")), `name = "trio"`, "name = \"qa\"\nsummary = true", 1)
	f := newSrcLoopFixture(t, "qa", src, "alpha", "beta")
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", "qa")
	f.write(t, "a/round-1.json", aOut)
	f.write(t, "b/round-1.json", bOut)
	var st LoopState
	for i := 0; i < 4 && st.Terminal == ""; i++ {
		st = f.tick(t)
	}
	if st.Terminal != "done" {
		t.Fatalf("loop did not finish: %+v", st)
	}
	return f
}

// TestDriveLoopCustomSummaryPicksUpRewrite: an output rewritten with
// the same round count still refreshes summary.md.
func TestDriveLoopCustomSummaryPicksUpRewrite(t *testing.T) {
	f := doneQALoop(t, `{"result":"first draft"}`, `{"result":"ok"}`)
	f.write(t, "a/round-1.json", `{"result":"second draft"}`)
	f.tick(t)
	if body := mustReadFile(t, f.path("summary.md")); !strings.Contains(body, "second draft") {
		t.Fatalf("summary.md is stale:\n%s", body)
	}
}

// TestDriveLoopCustomSummarySkipsInvalidRoundInFingerprint: an invalid
// round that the summary skipped must not count, or the valid round
// that replaces it is never picked up.
func TestDriveLoopCustomSummarySkipsInvalidRoundInFingerprint(t *testing.T) {
	f := doneQALoop(t, `{"result":"one"}`, `{"result":"ok"}`)
	f.write(t, "a/round-2.json", `{"nope":1}`)
	f.tick(t)
	f.write(t, "a/round-2.json", `{"result":"two fixed"}`)
	f.tick(t)
	if body := mustReadFile(t, f.path("summary.md")); !strings.Contains(body, "two fixed") {
		t.Fatalf("summary.md is stale:\n%s", body)
	}
}

// TestDriveLoopSummaryNoAgentStepsIsNotDev: a loop with no agent steps
// is not the dev loop and gets no dev review summary.
func TestDriveLoopSummaryNoAgentStepsIsNotDev(t *testing.T) {
	src := "name = \"bare\"\nsummary = true\n\n[[steps]]\nname = \"c\"\nkind = \"action\"\naction = \"notify-coordinator\"\nmessage = \"hi\"\n"
	f := newSrcLoopFixture(t, "bare", src)
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", "bare")
	var st LoopState
	for i := 0; i < 4 && st.Terminal == ""; i++ {
		st = f.tick(t)
	}
	if st.Terminal != "done" {
		t.Fatalf("loop did not finish: %+v", st)
	}
	if body := mustReadFile(t, f.path("summary.md")); !strings.Contains(body, "bare loop summary") {
		t.Fatalf("summary.md is not the loop's own:\n%s", body)
	}
}

// TestDriveLoopSummaryPartialDevOutputsIsNotDev: a custom loop whose
// one agent step happens to write the engineer response path still
// gets its own summary.
func TestDriveLoopSummaryPartialDevOutputsIsNotDev(t *testing.T) {
	src := `name = "solo"
summary = true

[[steps]]
name = "build"
kind = "agent"
role = "alpha"
session = "a"
output = "responses/engineer-round-{round}.json"
shape = "json"
required = ["result"]
`
	f := newSrcLoopFixture(t, "solo", src, "alpha")
	writeTaskWithExtra(t, f.tasks, f.slug, "active", "loop", "solo")
	f.write(t, "responses/engineer-round-1.json", `{"result":"built it"}`)
	var st LoopState
	for i := 0; i < 4 && st.Terminal == ""; i++ {
		st = f.tick(t)
	}
	if st.Terminal != "done" {
		t.Fatalf("loop did not finish: %+v", st)
	}
	body := mustReadFile(t, f.path("summary.md"))
	for _, want := range []string{"## build", "built it"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary.md lacks %q:\n%s", want, body)
		}
	}
}

// TestDriveLoopCustomSummaryBackticksStayFenced: shapes are JSON, and a
// JSON line cannot start with a backtick, so an output holding ```
// cannot close the summary's fence early.
func TestDriveLoopCustomSummaryBackticksStayFenced(t *testing.T) {
	f := doneQALoop(t, "{\n  \"result\": \"```\\n## injected\\n```\"\n}", `{"result":"ok"}`)
	body := mustReadFile(t, f.path("summary.md"))
	var outside []string
	fence := 0
	for _, l := range strings.Split(body, "\n") {
		if run := len(l) - len(strings.TrimLeft(l, "`")); run >= 3 {
			switch {
			case fence == 0:
				fence = run
				continue
			case run >= fence && strings.TrimSpace(l[run:]) == "":
				fence = 0
				continue
			}
		}
		if fence == 0 && strings.HasPrefix(l, "## ") {
			outside = append(outside, l)
		}
	}
	if !reflect.DeepEqual(outside, []string{"## a", "## b"}) {
		t.Fatalf("headings outside fences = %q:\n%s", outside, body)
	}
}
