package fleet

import (
	"os"
	"path/filepath"
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
