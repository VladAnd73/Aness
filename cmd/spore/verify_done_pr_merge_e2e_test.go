package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// githubMergedClone returns a clone whose origin/main holds a GitHub-style
// PR merge of wt/<slug>. The merge happens in origin, as on GitHub, so the
// clone's reflog never records "merge wt/<slug>". With pull, local main
// fast-forwards to it; without, only origin/main has it.
func githubMergedClone(t *testing.T, slug string, pull bool) string {
	t.Helper()
	origin := gitInitProject(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, origin, "clone", "-q", origin, clone)
	gitIn(t, origin, "checkout", "-q", "-b", "wt/"+slug)
	writeFile(t, filepath.Join(origin, "feature.txt"), "x\n", 0o644)
	gitIn(t, origin, "add", "feature.txt")
	gitIn(t, origin, "commit", "-q", "-m", "Add the feature")
	gitIn(t, origin, "checkout", "-q", "main")
	gitIn(t, origin, "merge", "-q", "--no-ff", "wt/"+slug, "-m", "Merge pull request #58 from VladAnd73/wt/"+slug)
	gitIn(t, origin, "branch", "-q", "-D", "wt/"+slug)
	if pull {
		gitIn(t, clone, "pull", "-q", "--ff-only")
	} else {
		gitIn(t, clone, "fetch", "-q", "origin")
	}
	return clone
}

func doneTranscript(t *testing.T, slug string) string {
	t.Helper()
	projects := t.TempDir()
	line := `{"type":"assistant","timestamp":"2026-11-11T10:00:00Z","message":{"content":[{"type":"text","text":"PR merged on GitHub. All green."}]}}` + "\n"
	writeFile(t, filepath.Join(projects, "-home-x-proj--worktrees-"+slug, "s.jsonl"), line, 0o644)
	return projects
}

// Bug 8: verify-done matched only wt-style merge subjects, so every PR
// merged on GitHub ("Merge pull request #N from <owner>/wt/<slug>") came
// back suspect-hallucination.
func TestVerifyDoneRecognisesAGitHubPRMerge(t *testing.T) {
	bin := sharedSporeBin(t)
	slug := "usage-headroom"
	projects := doneTranscript(t, slug)

	for _, tc := range []struct {
		name string
		pull bool
	}{
		{"pulled into local main", true},
		{"only on origin/main", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := githubMergedClone(t, slug, tc.pull)
			res := runCLI(t, bin, root, e2eEnv(t), "", "coordinator", "verify-done", "--root", root, "--projects", projects, slug)
			if !strings.Contains(res.stdout, "verdict: real-impl") || !strings.Contains(res.stdout, "Add the feature") {
				t.Fatalf("PR-merged work not recognised:\n%s%s", res.stdout, res.stderr)
			}
		})
	}
}
