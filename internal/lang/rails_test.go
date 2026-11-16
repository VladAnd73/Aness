package lang

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestIsRails_BothMarkersPresent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Gemfile"))
	writeFile(t, filepath.Join(root, "config", "application.rb"))
	if !IsRails(root) {
		t.Fatalf("IsRails(%s) = false; want true", root)
	}
}

func TestIsRails_GemfileOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Gemfile"))
	if IsRails(root) {
		t.Fatalf("IsRails(%s) = true; Gemfile alone should not match (plain Ruby gem)", root)
	}
}

func TestIsRails_ApplicationRbOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config", "application.rb"))
	if IsRails(root) {
		t.Fatalf("IsRails(%s) = true; config/application.rb alone should not match", root)
	}
}

func TestIsRails_EmptyRoot(t *testing.T) {
	if IsRails("") {
		t.Fatalf("IsRails(\"\") = true; want false")
	}
}

func TestIsRails_GemfileIsDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Gemfile"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(root, "config", "application.rb"))
	if IsRails(root) {
		t.Fatalf("IsRails: directory named Gemfile should not satisfy the file marker")
	}
}
