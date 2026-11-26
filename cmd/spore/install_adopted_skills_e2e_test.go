package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var adoptedSkills = []string{
	"reviewing-against-the-brief",
	"running-a-harness-retro",
	"building-a-repro-loop",
}

func TestInstallCLIShipsAdoptedSkills(t *testing.T) {
	bin := sharedSporeBin(t)
	root := t.TempDir()
	res := runCLI(t, bin, root, e2eEnv(t), "", "install", "--root", root)
	if res.code != 0 {
		t.Fatalf("spore install exit=%d stderr=%s", res.code, res.stderr)
	}
	for _, name := range adoptedSkills {
		body, err := os.ReadFile(filepath.Join(root, ".claude", "skills", name, "SKILL.md"))
		if err != nil {
			t.Errorf("skill %s not installed: %v", name, err)
			continue
		}
		fm := skillFrontmatter(string(body))
		if fm["name"] != name {
			t.Errorf("skill %s: frontmatter name=%q", name, fm["name"])
		}
		if desc := fm["description"]; !strings.HasPrefix(desc, "Use when") {
			t.Errorf("skill %s: description must start with %q, got %q", name, "Use when", desc)
		}
	}
}

func skillFrontmatter(body string) map[string]string {
	out := map[string]string{}
	rest, ok := strings.CutPrefix(body, "---\n")
	if !ok {
		return out
	}
	head, _, _ := strings.Cut(rest, "\n---\n")
	for _, line := range strings.Split(head, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(line, " ") {
			out[k] = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return out
}
