package main

import (
	"os"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

func TestRunTaskMergeForceMergeRedRequiresReason(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"flag with no follow-up", []string{"demo", "--force-merge-red"}},
		{"flag followed by empty string", []string{"demo", "--force-merge-red", ""}},
		{"flag with empty equals", []string{"demo", "--force-merge-red="}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runTaskMerge(tc.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "requires a <reason>") {
				t.Errorf("error = %q should mention requires a <reason>", err)
			}
		})
	}
}

func TestRunTaskMergeUnknownFlag(t *testing.T) {
	err := runTaskMerge([]string{"demo", "--bogus"})
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q should mention unknown flag", err)
	}
}

func TestRunTaskMergeMissingSlug(t *testing.T) {
	err := runTaskMerge(nil)
	if err == nil {
		t.Fatal("expected error for missing slug, got nil")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q should be a usage hint", err)
	}
}

func TestRunTaskNewRoleRefusesStart(t *testing.T) {
	t.Chdir(t.TempDir())
	err := runTaskNew([]string{"--role", "--start", "--no-edit", "demo"})
	if err == nil {
		t.Fatal("expected error for --role with --start, got nil")
	}
	if !strings.Contains(err.Error(), "--role does not combine with --start") {
		t.Errorf("error = %q should mention --role does not combine with --start", err)
	}
	if _, statErr := os.Stat("tasks"); !os.IsNotExist(statErr) {
		t.Errorf("tasks dir created despite refusal: %v", statErr)
	}
}

func TestRunTaskNewRoleWritesLoop(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runTaskNew([]string{"--role", "--no-edit", "demo"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("tasks/demo.md")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := frontmatter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.Loop != task.LoopRole || m.Status != "draft" {
		t.Errorf("meta = loop %q status %q, want %q draft", m.Loop, m.Status, task.LoopRole)
	}
}

func TestRunTaskRoleDriveUsage(t *testing.T) {
	err := runTask([]string{"role-drive"})
	if err == nil || !strings.Contains(err.Error(), "usage: spore task role-drive <slug>") {
		t.Errorf("err = %v, want role-drive usage", err)
	}
}

func TestRunTaskStatusUsage(t *testing.T) {
	err := runTask([]string{"status"})
	if err == nil || !strings.Contains(err.Error(), "usage: spore task status <slug>") {
		t.Errorf("err = %v, want status usage", err)
	}
}
