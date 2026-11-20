package research

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// buildScratch makes the editor's whole world for one try: the target,
// the rest of the skill folder under frozen/, goal.md, and the score
// log cut down to editorColumns.
func buildScratch(s Store, cfg Config, night, try int) (dir string, err error) {
	dir = filepath.Join(s.Dir(cfg.Name), "nights", fmt.Sprint(night), fmt.Sprintf("try-%d", try), "scratch")
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	frozen := filepath.Join(dir, "frozen")
	if err := os.MkdirAll(frozen, 0o700); err != nil {
		return "", err
	}
	repoDir := s.repo(cfg.Name).dir
	rel := cfg.targetRel()
	err = filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r, err := filepath.Rel(repoDir, path)
		if err != nil || r == "." {
			return err
		}
		if r == ".git" {
			return filepath.SkipDir
		}
		out := filepath.Join(frozen, r)
		if r == rel {
			out = filepath.Join(dir, rel)
		}
		if d.IsDir() {
			return os.MkdirAll(out, 0o700)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o600)
	})
	if err != nil {
		return "", fmt.Errorf("research: scratch: %w", err)
	}
	goal, err := os.ReadFile(cfg.GoalFile)
	if err != nil {
		return "", fmt.Errorf("research: scratch: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "goal.md"), goal, 0o600); err != nil {
		return "", err
	}
	rows, err := ReadRows(filepath.Join(s.Dir(cfg.Name), "results.tsv"))
	if err != nil {
		return "", err
	}
	return dir, writeEditorResults(filepath.Join(dir, "results.tsv"), rows)
}
