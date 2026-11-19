package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/versality/spore/internal/research"
)

// roleEditor teaches the fixture role one of the two main-set bugs on
// try 2 only, so the night stays under 100 and ends on max-tries. Try 1
// adds a blank line and try 3 a trailing note: no gain, reset.
type roleEditor struct{ calls int }

func (e *roleEditor) Propose(ctx context.Context, req research.EditRequest) (research.EditResult, error) {
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
		b = bytes.ReplaceAll(b, []byte("cover: happy-path"), []byte("cover: empty-title"))
	case 3:
		b = append(b, "Keep the plan short.\n"...)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return research.EditResult{ExitCode: 1}, err
	}
	return research.EditResult{Description: fmt.Sprintf("try %d", e.calls)}, nil
}

// Acceptance 14: a role campaign runs new -> judge approve -> on ->
// night -> stop -> accept through the built binary, and the kept edit
// lands on research/<c> in the role's own repo.
func TestAcceptanceRole14EndToEnd(t *testing.T) {
	bin := buildSporeBin(t)
	_, goal := roleResearchEnv(t)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "1")
	fixture, err := filepath.Abs("testdata/research-roles-e2e")
	if err != nil {
		t.Fatal(err)
	}
	proj := copyFixtureProject(t)
	target := filepath.Join(proj, "bootstrap", "roles", "qa-analyst.md")
	original, _ := os.ReadFile(target)
	campaignDir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "spore", "research", "rc")

	editor := &roleEditor{}
	deps := researchDeps{
		Editor:      editor,
		Room:        func() (int, string) { return 0, "room" },
		WeeklyUsed:  func(context.Context) (float64, error) { return 20.0, nil },
		Notify:      func(string) error { return nil },
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

	run("new", "rc", "--target", target, "--kind", "role", "--step", "intake", "--goal", goal)
	setCampaignKeys(t, filepath.Join(campaignDir, "campaign.toml"), map[string]string{
		"max_tries_per_night": "3",
		"repeats":             "1",
		"isolation":           `"none"`,
		"agent_binary":        fmt.Sprintf("%q", filepath.Join(fixture, "fake-claude.sh")),
	})
	run("judge", "approve", "rc", "--from", filepath.Join(fixture, "drafts"))
	run("on", "rc")
	run("night", "rc")
	run("stop", "rc")
	acceptOut := run("accept", "rc")

	results, _ := os.ReadFile(filepath.Join(campaignDir, "results.tsv"))
	if got := tryStatuses(string(results)); strings.Join(got, ",") != "reset,kept,reset" {
		t.Errorf("statuses for tries 1..3 = %v, want [reset kept reset]\n%s", got, results)
	}
	report, _ := os.ReadFile(filepath.Join(campaignDir, "report.md"))
	for _, want := range []string{`start score:\s*0\.0\b`, `best score:\s*50\.0\b`, `holdout:\s*pass\b`} {
		if !regexp.MustCompile(want).Match(report) {
			t.Errorf("report.md does not match %q:\n%s", want, report)
		}
	}
	if !strings.Contains(acceptOut, "branch: research/rc") {
		t.Errorf("accept output %q names no research/rc branch", acceptOut)
	}
	out, err := exec.Command("git", "-C", proj, "show", "research/rc:bootstrap/roles/qa-analyst.md").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "cover: empty-title") {
		t.Errorf("research/rc role body = %q, %v; want the kept edit", out, err)
	}
	if live, _ := os.ReadFile(target); !bytes.Equal(live, original) {
		t.Errorf("the live role body changed: %q", live)
	}
}
