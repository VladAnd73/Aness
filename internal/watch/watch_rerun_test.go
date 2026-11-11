package watch

import (
	"os"
	"path/filepath"
	"testing"
)

// rerunScript serves PR #7 at the head SHA in $DIR/sha with the check
// state in $DIR/state, so a test can walk one PR through CI reruns and
// new pushes.
func rerunScript(dir string) string {
	return `
case "$2" in
list)
  if [ -f ` + dir + `/closed ]; then echo '[]'; exit 0; fi
  echo '[{"number":7,"headRefName":"feat-x","headRefOid":"'$(cat ` + dir + `/sha)'","isDraft":false,"url":"u7"}]' ;;
checks) echo '[{"name":"e2e-run","bucket":"'$(cat ` + dir + `/state)'","link":"l"}]'; exit 8 ;;
esac`
}

func setRerun(t *testing.T, dir, sha, state string) {
	t.Helper()
	for name, v := range map[string]string{"sha": sha, "state": state} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunDedupSurvivesACIRerunOnTheSameCommit(t *testing.T) {
	dir := t.TempDir()
	root, tells, tell := setupRun(t, rerunScript(dir))
	steps := []struct {
		sha, state string
		wantTotal  int
	}{
		{"sha1", "fail", 1},
		{"sha1", "pending", 1},
		{"sha1", "fail", 1},
		{"sha1", "pass", 1},
		{"sha1", "fail", 1},
		{"sha2", "fail", 2},
	}
	for i, s := range steps {
		setRerun(t, dir, s.sha, s.state)
		if _, err := Run(root, "proj", false, tell); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if len(*tells) != s.wantTotal {
			t.Fatalf("step %d (%s %s): %d alert(s) so far, want %d", i, s.sha, s.state, len(*tells), s.wantTotal)
		}
	}
}

func TestRunPrunesKeysOfAClosedPRAndAnOldHead(t *testing.T) {
	dir := t.TempDir()
	root, _, tell := setupRun(t, rerunScript(dir))
	setRerun(t, dir, "sha1", "fail")
	if _, err := Run(root, "proj", false, tell); err != nil {
		t.Fatal(err)
	}
	setRerun(t, dir, "sha2", "pending")
	if _, err := Run(root, "proj", false, tell); err != nil {
		t.Fatal(err)
	}
	st, err := LoadState("proj")
	if err != nil {
		t.Fatal(err)
	}
	if st.SeenKey(Key(7, "sha1", "e2e-run")) {
		t.Fatal("the key of a superseded head SHA survived the prune")
	}
	setRerun(t, dir, "sha2", "fail")
	if _, err := Run(root, "proj", false, tell); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "closed"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, "proj", false, tell); err != nil {
		t.Fatal(err)
	}
	st, _ = LoadState("proj")
	if n := len(st.Seen); n != 0 {
		t.Fatalf("%d seen key(s) left after the PR closed: %v", n, st.Seen)
	}
}
