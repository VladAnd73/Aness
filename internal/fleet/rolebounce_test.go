package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task"
)

type wakeCall struct {
	session, stem, msg string
}

func stubWakeRole(t *testing.T, outcome *wakeOutcome) *[]wakeCall {
	t.Helper()
	var calls []wakeCall
	prev := wakeRole
	wakeRole = func(spec RoleSpawnSpec, stem, msg string) (wakeOutcome, error) {
		s, _ := spec.SessionName()
		calls = append(calls, wakeCall{s, stem, msg})
		return *outcome, nil
	}
	t.Cleanup(func() { wakeRole = prev })
	return &calls
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An invalid verdict must not count: the loop stays in review-A
// instead of erroring (wedge) or advancing on an unknown value (skew).
func TestDeriveSnapshotSkipsInvalidFiles(t *testing.T) {
	root := t.TempDir()
	slug := "demo"
	if err := task.EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	if err := task.WriteEngineerResponse(root, slug, 1, task.EngineerResponse{}); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, task.ReviewPath(root, slug, task.ReviewerA, 1), `{"author":"A","verdict":"approved","summary":"s"}`)
	writeRaw(t, task.EngineerResponsePath(root, slug, 2), "not json")

	snap, err := DeriveSnapshot(root, slug)
	if err != nil {
		t.Fatalf("DeriveSnapshot: %v", err)
	}
	if snap.Phase != PhaseReviewA || snap.ReviewerRound != 0 || snap.EngineerRound != 1 {
		t.Errorf("snap = %+v, want review-A round 0, engineer 1", snap)
	}
}

func TestBounceWaitsForWakeThenMovesAside(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	stateDir := t.TempDir()
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", stateDir)
	slug := "demo"
	if err := task.EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	path := task.ReviewPath(root, slug, task.ReviewerB, 2)
	writeRaw(t, path, `{"author":"B","verdict":"approve","summary":"s","severity":"low"}`)

	outcome := wakePending
	calls := stubWakeRole(t, &outcome)
	for range 2 {
		if err := bounceInvalidRoleFiles(root, slug); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file moved before the wake was delivered: %v", err)
	}
	if len(*calls) != 2 {
		t.Fatalf("wake calls = %d, want 2 (retry each tick)", len(*calls))
	}
	c := (*calls)[0]
	if !strings.HasSuffix(c.session, "/demo/reviewer-B") {
		t.Errorf("woke %s, want reviewer B", c.session)
	}
	for _, want := range []string{path, `unknown field "severity"`, "round-2.json.invalid", "Write tool", `"author": "B"`} {
		if !strings.Contains(c.msg, want) {
			t.Errorf("wake msg missing %q: %s", want, c.msg)
		}
	}
	if strings.Contains(c.msg, "\n") {
		t.Errorf("wake msg has a newline: %q", c.msg)
	}

	outcome = wakeDelivered
	if err := bounceInvalidRoleFiles(root, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still in place after delivery: %v", err)
	}
	if _, err := os.Stat(path + task.InvalidSuffix); err != nil {
		t.Errorf("quarantined copy missing: %v", err)
	}

	envs := readInboxEnvelopes(t, filepath.Join(stateDir, "demo-project", "inbox"))
	if len(envs) != 1 {
		t.Fatalf("coordinator envelopes = %d, want 1: %+v", len(envs), envs)
	}
	if !strings.Contains(envs[0].Body, "demo: invalid reviews/B/round-2.json") {
		t.Errorf("envelope body = %q", envs[0].Body)
	}

	// The same bad content written again is a new bounce: fresh wake
	// stem and a second coordinator notice.
	writeRaw(t, path, `{"author":"B","verdict":"approve","summary":"s","severity":"low"}`)
	if err := bounceInvalidRoleFiles(root, slug); err != nil {
		t.Fatal(err)
	}
	if last := (*calls)[len(*calls)-1]; last.stem == c.stem {
		t.Errorf("re-bounce reused wake stem %s", c.stem)
	}
	if _, err := os.Stat(path + task.InvalidSuffix + ".2"); err != nil {
		t.Errorf("second quarantine missing: %v", err)
	}
	if envs := readInboxEnvelopes(t, filepath.Join(stateDir, "demo-project", "inbox")); len(envs) != 2 {
		t.Errorf("coordinator envelopes = %d, want 2", len(envs))
	}
}

func TestBounceMovesAsideWhenAuthorPaneAbsent(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", t.TempDir())
	slug := "demo"
	if err := task.EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	path := task.EngineerResponsePath(root, slug, 1)
	writeRaw(t, path, `{"addressed":[]}`)

	outcome := wakeNoSession
	calls := stubWakeRole(t, &outcome)
	if err := bounceInvalidRoleFiles(root, slug); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || !strings.HasSuffix((*calls)[0].session, "/demo/engineer") {
		t.Fatalf("calls = %+v, want one engineer wake", *calls)
	}
	if !strings.Contains((*calls)[0].msg, `"author": "engineer"`) {
		t.Errorf("msg = %s", (*calls)[0].msg)
	}
	if _, err := os.Stat(path + task.InvalidSuffix); err != nil {
		t.Errorf("quarantined copy missing: %v", err)
	}
}

func TestBounceLeavesGrandfatheredFilesAlone(t *testing.T) {
	root := newGitRepoFor(t, "demo-project")
	slug := "demo"
	path := task.ReviewPath(root, slug, task.ReviewerA, 1)
	writeRaw(t, path, `{"verdict":"request_changes","summary":"pre-upgrade"}`)
	if err := task.EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	outcome := wakeDelivered
	calls := stubWakeRole(t, &outcome)
	if err := bounceInvalidRoleFiles(root, slug); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("grandfathered file bounced: %+v", *calls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("grandfathered file moved: %v", err)
	}
}
