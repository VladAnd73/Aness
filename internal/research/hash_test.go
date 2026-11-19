package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hashFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o700)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("alpha"), 0o600)
	os.WriteFile(filepath.Join(dir, "sub", "b.sh"), []byte("beta"), 0o700)
	return dir
}

func TestHashTreeStableAcrossRuns(t *testing.T) {
	dir := hashFixture(t)
	a, err := HashTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := HashTree(dir)
	if a != b || !strings.HasPrefix(a, "sha256:") {
		t.Errorf("HashTree = %q then %q, want equal sha256: values", a, b)
	}
}

func TestHashTreeChangesOnAnyByte(t *testing.T) {
	dir := hashFixture(t)
	base, _ := HashTree(dir)
	edits := map[string]func(){
		"content": func() { os.WriteFile(filepath.Join(dir, "a.md"), []byte("alphA"), 0o600) },
		"exec":    func() { os.Chmod(filepath.Join(dir, "a.md"), 0o700) },
		"rename":  func() { os.Rename(filepath.Join(dir, "a.md"), filepath.Join(dir, "c.md")) },
		"add":     func() { os.WriteFile(filepath.Join(dir, "new"), nil, 0o600) },
	}
	for name, edit := range edits {
		dir = hashFixture(t)
		edit()
		got, err := HashTree(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got == base {
			t.Errorf("%s: hash did not change", name)
		}
	}
}

func TestHashFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	os.WriteFile(p, []byte("x"), 0o600)
	a, err := HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p, []byte("y"), 0o600)
	if b, _ := HashFile(p); a == b {
		t.Errorf("HashFile did not change with content")
	}
}
