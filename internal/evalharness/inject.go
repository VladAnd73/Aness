package evalharness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InjectSkill installs a candidate skill where the agent under test
// loads project skills: frozenDir (the rest of the skill folder, may be
// empty) is copied to <runWD>/.claude/skills/<skillName>/, then the
// candidate file is written over its SKILL.md.
func InjectSkill(runWD, frozenDir, skillName, candidate string) error {
	if err := checkSkillName(skillName); err != nil {
		return err
	}
	body, err := os.ReadFile(candidate)
	if err != nil {
		return fmt.Errorf("evalharness: inject: candidate: %w", err)
	}
	dst := filepath.Join(runWD, ".claude", "skills", skillName)
	if frozenDir != "" {
		if err := copyTree(frozenDir, dst); err != nil {
			return fmt.Errorf("evalharness: inject: frozen folder: %w", err)
		}
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("evalharness: inject: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), body, 0o644); err != nil {
		return fmt.Errorf("evalharness: inject: %w", err)
	}
	return nil
}

func checkSkillName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || !filepath.IsLocal(name) {
		return fmt.Errorf("evalharness: skill name %q must be a single path element", name)
	}
	return nil
}

// copyTree copies regular files and directories from src into dst. A
// symlink is refused: it could point the agent under test at a file
// outside the frozen folder.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, b, 0o644)
		default:
			return fmt.Errorf("%s: only regular files and directories are copied", path)
		}
	})
}
