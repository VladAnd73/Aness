package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListUnparseableNamesBrokenTaskFilesOnly(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"README.md": "# tasks\n\nnot a task\n",
		"good.md":   "---\nstatus: active\nslug: good\n---\nbody\n",
		"gamma.md":  "---\nstatus: active\nslug: gamma\n# reviewer: keep\n---\nbody\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bad, err := ListUnparseable(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 || bad[0].Slug != "gamma" || bad[0].Err == nil {
		t.Fatalf("ListUnparseable = %+v, want only gamma", bad)
	}
	metas, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Slug != "good" {
		t.Fatalf("List = %+v, want only good", metas)
	}
}
