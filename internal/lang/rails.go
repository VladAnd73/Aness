// Package lang holds per-language project-kind detectors. Each function
// returns true when projectRoot looks like a project of that kind. The
// detectors feed the composer's predicate map ("?rails", "?<other>")
// so language-specific rule fragments only render for matching trees.
package lang

import (
	"os"
	"path/filepath"
)

// IsRails reports whether projectRoot looks like a Ruby on Rails app.
// Both markers must be present: a Gemfile at the root and a
// config/application.rb file (the canonical Rails app entry point).
// The Gemfile alone matches plain Ruby gems too; requiring
// config/application.rb narrows to actual Rails apps.
func IsRails(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	if !regularFile(filepath.Join(projectRoot, "Gemfile")) {
		return false
	}
	return regularFile(filepath.Join(projectRoot, "config", "application.rb"))
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
