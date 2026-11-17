package editor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

func addWorktree(t *testing.T, root, slug string) {
	t.Helper()
	gitRun(t, root, "worktree", "add", "-q", "-b", "wt/"+slug, filepath.Join(root, ".worktrees", slug))
}

func parseWorkspace(t *testing.T, data []byte) workspaceFile {
	t.Helper()
	var ws workspaceFile
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	return ws
}

func TestRenderWorkspaceFolders(t *testing.T) {
	root := newRepo(t)
	addWorktree(t, root, "zeta")
	addWorktree(t, root, "alpha")
	if err := os.MkdirAll(filepath.Join(root, ".worktrees", "stray"), 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := RenderWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ws := parseWorkspace(t, data)
	want := []workspaceFolder{
		{Name: "proj", Path: ".."},
		{Name: "alpha", Path: "../.worktrees/alpha"},
		{Name: "zeta", Path: "../.worktrees/zeta"},
	}
	if len(ws.Folders) != len(want) {
		t.Fatalf("folders = %+v, want %+v", ws.Folders, want)
	}
	for i := range want {
		if ws.Folders[i] != want[i] {
			t.Errorf("folder %d = %+v, want %+v", i, ws.Folders[i], want[i])
		}
		abs := filepath.Join(filepath.Dir(WorkspacePath(root)), ws.Folders[i].Path)
		if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
			t.Errorf("folder %q does not resolve to a checkout: %v", ws.Folders[i].Path, err)
		}
	}
	for _, key := range []string{"files.watcherExclude", "search.exclude"} {
		m, ok := ws.Settings[key].(map[string]any)
		if !ok || m["**/.worktrees/**"] != true {
			t.Errorf("settings[%q] = %v, want **/.worktrees/** excluded", key, ws.Settings[key])
		}
	}
	if len(ws.Settings) != 2 {
		t.Errorf("settings = %v, want only the two excludes", ws.Settings)
	}
}

func TestRenderWorkspaceNoWorktrees(t *testing.T) {
	root := newRepo(t)
	data, err := RenderWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ws := parseWorkspace(t, data)
	if len(ws.Folders) != 1 || ws.Folders[0].Path != ".." {
		t.Errorf("folders = %+v, want main checkout only", ws.Folders)
	}
}

func TestWriteWorkspaceNoOpWhenUnchanged(t *testing.T) {
	root := newRepo(t)
	addWorktree(t, root, "alpha")

	path, changed, err := WriteWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || path != filepath.Join(root, ".spore", "spore.code-workspace") {
		t.Fatalf("first write: path=%s changed=%v", path, changed)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	_, changed, err = WriteWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second write reported a change on identical content")
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(old) {
		t.Errorf("file rewritten on identical content: %v %v", fi.ModTime(), err)
	}

	addWorktree(t, root, "beta")
	_, changed, err = WriteWorkspace(root)
	if err != nil || !changed {
		t.Fatalf("write after new worktree: changed=%v err=%v", changed, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf(".spore/ holds %d entries, want only the workspace (temp file leaked?)", len(entries))
	}
}

func TestRefreshWorkspaceOptIn(t *testing.T) {
	root := newRepo(t)
	addWorktree(t, root, "alpha")

	changed, err := RefreshWorkspace(root)
	if err != nil || changed {
		t.Fatalf("refresh without file: changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(WorkspacePath(root)); !os.IsNotExist(err) {
		t.Fatalf("refresh created the file: %v", err)
	}

	if _, _, err := WriteWorkspace(root); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "worktree", "remove", "--force", filepath.Join(root, ".worktrees", "alpha"))
	changed, err = RefreshWorkspace(root)
	if err != nil || !changed {
		t.Fatalf("refresh after worktree remove: changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(WorkspacePath(root))
	if ws := parseWorkspace(t, data); len(ws.Folders) != 1 {
		t.Errorf("folders after remove = %+v, want main only", ws.Folders)
	}
}
