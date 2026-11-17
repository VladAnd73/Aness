package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// WorkspaceRel is the multi-root VS Code workspace path relative to
// the main checkout. It sits in .spore/ so folder paths are `..` and
// `../.worktrees/<slug>`, valid wherever the repo lives.
const WorkspaceRel = ".spore/spore.code-workspace"

type workspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// No git.* settings: each folder is its own repo root, and the main
// folder's default scan depth (1) stops at .worktrees/ itself, so
// Source Control already lists one repo per folder.
type workspaceFile struct {
	Folders  []workspaceFolder `json:"folders"`
	Settings map[string]any    `json:"settings"`
}

func WorkspacePath(root string) string {
	return filepath.Join(root, WorkspaceRel)
}

// RenderWorkspace builds the workspace JSON for the main checkout at
// root: the checkout first, then each .worktrees/<slug> that git lists
// as a worktree of this repo, sorted by slug. Output is deterministic
// so regenerating an unchanged tree is a byte-for-byte no-op.
func RenderWorkspace(root string) ([]byte, error) {
	slugs, err := worktreeSlugs(root)
	if err != nil {
		return nil, err
	}
	folders := []workspaceFolder{{Name: filepath.Base(root), Path: ".."}}
	for _, s := range slugs {
		folders = append(folders, workspaceFolder{Name: s, Path: "../.worktrees/" + s})
	}
	// Worktrees are added back as their own roots; excluding them
	// from the main folder keeps its watcher and search off ~70% of
	// the files on a busy consumer.
	exclude := map[string]bool{"**/.worktrees/**": true}
	ws := workspaceFile{
		Folders: folders,
		Settings: map[string]any{
			"files.watcherExclude": exclude,
			"search.exclude":       exclude,
		},
	}
	out, err := json.MarshalIndent(ws, "", "\t")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// WriteWorkspace renders the workspace and writes it only when the
// bytes differ, via temp file plus rename (VS Code watches the file).
func WriteWorkspace(root string) (path string, changed bool, err error) {
	path = WorkspacePath(root)
	data, err := RenderWorkspace(root)
	if err != nil {
		return path, false, err
	}
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return path, false, nil
	}
	if err := writeAtomic(path, data); err != nil {
		return path, false, err
	}
	return path, true, nil
}

// RefreshWorkspace regenerates the workspace only if it already
// exists. The operator opts in by running `spore code workspace` once;
// projects that never use an editor never get the file.
func RefreshWorkspace(root string) (changed bool, err error) {
	if _, err := os.Stat(WorkspacePath(root)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_, changed, err = WriteWorkspace(root)
	return changed, err
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".spore.code-workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// worktreeSlugs lists .worktrees/<slug> dirs that `git worktree list`
// reports for root, skipping stray dirs left by a failed remove.
func worktreeSlugs(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".worktrees"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	known, err := gitWorktrees(root)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := filepath.EvalSymlinks(filepath.Join(root, ".worktrees", e.Name()))
		if err != nil || !known[p] {
			continue
		}
		slugs = append(slugs, e.Name())
	}
	sort.Strings(slugs)
	return slugs, nil
}

func gitWorktrees(root string) (map[string]bool, error) {
	cmd := exec.Command("git", "-C", root, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list in %s: %w", root, err)
	}
	known := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		p, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		known[p] = true
	}
	return known, nil
}
