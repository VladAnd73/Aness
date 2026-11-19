package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/versality/spore/internal/research"
)

// scriptedEditor fixes the e2e skill's defect on try 2 only. Try 1
// adds a blank line (same score, longer: reset); try 3 adds a comment
// (same best score, longer: reset).
type scriptedEditor struct{ calls int }

func (e *scriptedEditor) Propose(ctx context.Context, req research.EditRequest) (research.EditResult, error) {
	e.calls++
	path := filepath.Join(req.ScratchDir, req.TargetRel)
	b, err := os.ReadFile(path)
	if err != nil {
		return research.EditResult{ExitCode: 1}, err
	}
	switch e.calls {
	case 1:
		b = append(b, '\n')
	case 2:
		b = bytes.ReplaceAll(b, []byte("status: todo"), []byte("status: done"))
	case 3:
		b = append(b, "<!-- keep it short -->\n"...)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return research.EditResult{ExitCode: 1}, err
	}
	return research.EditResult{Description: fmt.Sprintf("try %d", e.calls)}, nil
}

// setCampaignKeys rewrites key = value lines in a flat campaign.toml,
// dropping any earlier line for the same key.
func setCampaignKeys(t *testing.T, path string, kv map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(path)
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(old), "\n"), "\n") {
		key, _, _ := strings.Cut(line, "=")
		if _, replaced := kv[strings.TrimSpace(key)]; !replaced && line != "" {
			lines = append(lines, line)
		}
	}
	for k, v := range kv {
		lines = append(lines, k+" = "+v)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptance10EndToEndNight(t *testing.T) {
	bin := buildSporeBin(t)
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "1")

	fixture, err := filepath.Abs("testdata/research-e2e")
	if err != nil {
		t.Fatal(err)
	}
	liveDir := filepath.Join(root, "live", "e2e")
	if err := os.MkdirAll(liveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(fixture, "skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	liveTarget := filepath.Join(liveDir, "SKILL.md")
	if err := os.WriteFile(liveTarget, original, 0o600); err != nil {
		t.Fatal(err)
	}
	goal := filepath.Join(root, "goal.md")
	if err := os.WriteFile(goal, []byte("Make the skill record the run as done.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	campaignDir := filepath.Join(root, "state", "spore", "research", "e2e")

	editor := &scriptedEditor{}
	notes := []string{}
	deps := researchDeps{
		Editor:      editor,
		Room:        func() (int, string) { return 0, "room" },
		WeeklyUsed:  func(context.Context) (float64, error) { return 20.0, nil },
		Notify:      func(msg string) error { notes = append(notes, msg); return nil },
		SporeBinary: bin,
	}
	run := func(args ...string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := researchMain(&out, &errOut, args, deps); code != 0 {
			t.Fatalf("spore research %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out.String(), errOut.String())
		}
		return out.String()
	}

	run("new", "e2e", "--target", liveTarget, "--goal", goal, "--objective", "maximize-score")
	setCampaignKeys(t, filepath.Join(campaignDir, "campaign.toml"), map[string]string{
		"max_tries_per_night": "3",
		"isolation":           `"none"`,
		"agent_binary":        fmt.Sprintf("%q", filepath.Join(fixture, "fake-claude.sh")),
	})
	run("judge", "approve", "e2e", "--from", filepath.Join(fixture, "drafts"))
	run("on", "e2e")
	run("night", "e2e")
	run("stop", "e2e")
	acceptOut := run("accept", "e2e")

	results, _ := os.ReadFile(filepath.Join(campaignDir, "results.tsv"))
	if got := tryStatuses(string(results)); strings.Join(got, ",") != "reset,kept,reset" {
		t.Errorf("results.tsv statuses for tries 1..3 = %v, want [reset kept reset]\n%s", got, results)
	}
	if editor.calls != 3 {
		t.Errorf("editor calls = %d, want 3", editor.calls)
	}

	report, _ := os.ReadFile(filepath.Join(campaignDir, "report.md"))
	for _, want := range []string{`max-tries`, `start score:\s*0\.0\b`, `best score:\s*100\.0\b`, `holdout:\s*pass\b`} {
		if !regexp.MustCompile(`(?i)` + want).Match(report) {
			t.Errorf("report.md does not match %q:\n%s", want, report)
		}
	}

	live, _ := os.ReadFile(liveTarget)
	if !bytes.Contains(live, []byte("status: done")) || bytes.Contains(live, []byte("status: todo")) {
		t.Errorf("live SKILL.md after accept = %q, want the fixed skill", live)
	}
	backupPath := ""
	for _, line := range strings.Split(acceptOut, "\n") {
		if p, ok := strings.CutPrefix(line, "backup: "); ok {
			backupPath = strings.TrimSpace(p)
		}
	}
	if backup, err := os.ReadFile(backupPath); err != nil || !bytes.Equal(backup, original) {
		t.Errorf("backup %q = %q, %v; want the original skill holding status: todo (accept stdout: %q)", backupPath, backup, err, acceptOut)
	}
}

// tryStatuses returns the status column of results.tsv rows for tries
// 1, 2 and 3 in order, read by header name.
func tryStatuses(tsv string) []string {
	lines := strings.Split(strings.TrimRight(tsv, "\n"), "\n")
	if len(lines) < 2 {
		return nil
	}
	col := map[string]int{}
	for i, h := range strings.Split(lines[0], "\t") {
		col[h] = i
	}
	var got []string
	for _, line := range lines[1:] {
		cells := strings.Split(line, "\t")
		ti, si := col["try"], col["status"]
		if ti >= len(cells) || si >= len(cells) {
			continue
		}
		switch cells[ti] {
		case "1", "2", "3":
			got = append(got, cells[si])
		}
	}
	return got
}
