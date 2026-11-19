package research

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/evalharness"
	"github.com/versality/spore/internal/fleet"
)

const roleLoopTOML = `name = "qa"

[[steps]]
name = "intake"
kind = "agent"
role = "qa-analyst"
output = "intake/round-{round}.json"
shape = "json"
required = ["summary", "plan_path"]
skills = ["plan-skill"]

[[steps]]
name = "review"
kind = "agent"
role = "qa-reviewer"
label = "R"
output = "review/round-{round}.json"
shape = "verdict"

[[steps.next]]
on = "request_changes"
to = "intake"

[[steps.next]]
on = "approve"
to = "done"
`

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@localhost", "-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRoleProject writes an invented project in its own git repo: a
// role body, the loop naming it, and one of the step's skills.
func newRoleProject(t *testing.T) (root, target string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root = filepath.Join(t.TempDir(), "proj")
	files := map[string]string{
		"bootstrap/roles/qa-analyst.md":      strings.Repeat("a", 99) + "\n",
		"bootstrap/roles/qa-reviewer.md":     "reviewer\n",
		"bootstrap/loops/qa.toml":            roleLoopTOML,
		".claude/skills/plan-skill/SKILL.md": "---\nname: plan-skill\n---\nplan\n",
		"README.md":                          "fake project\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, root, "init", "--quiet", "--initial-branch=trunk")
	gitIn(t, root, "config", "user.name", "operator")
	gitIn(t, root, "config", "user.email", "operator@localhost")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "--quiet", "-m", "init")
	return root, filepath.Join(root, "bootstrap", "roles", "qa-analyst.md")
}

func newRoleCampaign(t *testing.T, cfgEdits func(*Config)) (fixture, string) {
	t.Helper()
	root, target := newRoleProject(t)
	tmp := t.TempDir()
	store := Store{Root: filepath.Join(tmp, "state", "spore", "research")}
	goal := filepath.Join(tmp, "goal.md")
	os.WriteFile(goal, []byte("Find more bugs.\n"), 0o600)
	binary := filepath.Join(tmp, "spore-binary")
	os.WriteFile(binary, []byte("not a real binary\n"), 0o700)
	if _, _, err := store.CreateRole(fixtureName, target, goal, "maximize-score", RoleOptions{Step: "intake"}); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	approved, err := store.ApproveJudge(fixtureName, "testdata/drafts", binary, []string{binary, "eval", "judge"})
	if err != nil {
		t.Fatalf("ApproveJudge: %v", err)
	}
	if err := store.SetOn(fixtureName, true); err != nil {
		t.Fatalf("SetOn: %v", err)
	}
	if cfgEdits != nil {
		cfg, _ := store.Load(fixtureName)
		cfgEdits(&cfg)
		store.Save(cfg)
	}
	original, _ := os.ReadFile(target)
	f := fixture{store: store, name: fixtureName, liveDir: filepath.Dir(target), liveTarget: target, original: original, approved: approved, notes: new([]string)}
	return f, root
}

// The step note is a copy of fleet's; this pins the copy to the
// original for a json step and a verdict step.
func TestStepNoteMatchesFleet(t *testing.T) {
	def, err := fleet.ParseLoopDef("test", []byte(roleLoopTOML))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range def.Steps {
		spec := fleet.StepSpec("/p", "slug", def, s)
		if got := evalharness.StepNote(roleStepFor(def.Name, s), spec.OutputTemplate, 0); got != spec.Note {
			t.Errorf("step %s note:\n got %q\nwant %q", s.Name, got, spec.Note)
		}
	}
}

// Acceptance 9: a tie on the holdout is overfit, a strict gain passes,
// and an original already at 100 is no-headroom, which accept refuses
// without --force.
func TestAcceptanceRole9HoldoutMustBeat(t *testing.T) {
	for _, c := range []struct {
		original, winner float64
		want             string
	}{{80, 80, "overfit"}, {80, 85, "pass"}, {100, 100, "no-headroom"}} {
		f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
		_, d := keptNight(t, f, func(b []byte) float64 {
			if bytes.Equal(b, f.original) {
				return c.original
			}
			return c.winner
		})
		if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if got := f.load(t).HoldoutVerdict; got != c.want {
			t.Errorf("holdout %v vs %v: verdict %q, want %q", c.original, c.winner, got, c.want)
		}
		if c.want != "no-headroom" {
			continue
		}
		if _, err := Accept(f.store, f.name, false); err == nil || !strings.Contains(err.Error(), "no-headroom") {
			t.Errorf("accept without --force on no-headroom = %v, want a refusal naming no-headroom", err)
		}
		if _, err := Accept(f.store, f.name, true); err != nil {
			t.Errorf("accept --force on no-headroom: %v", err)
		}
	}
}

// Acceptance 10: a holdout side cached under one judge is run again
// once the holdout set or the binary changed.
func TestAcceptanceRole10HoldoutCacheKeyedByJudge(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	j, _ := keptNight(t, f, func(b []byte) float64 { return 50 })
	cfg := f.load(t)
	run := func() int {
		before := j.holdoutCalls
		if _, err := runHoldout(context.Background(), f.store, cfg, j); err != nil {
			t.Fatalf("runHoldout: %v", err)
		}
		return j.holdoutCalls - before
	}
	if n := run(); n != 4 {
		t.Fatalf("first holdout ran %d judgings, want 4", n)
	}
	if n := run(); n != 0 {
		t.Errorf("unchanged judge ran %d judgings, want the cache", n)
	}
	cfg.HoldoutHash = "re-approved-holdout"
	if n := run(); n != 4 {
		t.Errorf("new holdout set ran %d judgings, want 4", n)
	}
	cfg.BinaryHash = "re-approved-binary"
	if n := run(); n != 4 {
		t.Errorf("new binary ran %d judgings, want 4", n)
	}
}

// Acceptance 11: a baseline of 100 refuses the night as no-headroom
// before any editor run, and the next night is refused up front.
func TestAcceptanceRole11NoHeadroomNight(t *testing.T) {
	f := newCampaign(t, nil)
	ed := &fakeEditor{edits: []func(string) error{appendBytes("x\n")}}
	j := &fakeJudge{scores: []float64{100, 100}}
	d := f.deps(ed, j)
	_, err := RunNight(context.Background(), f.store, f.name, d)
	var refused *RefusedError
	if !errors.As(err, &refused) || !strings.Contains(refused.Reason, "no-headroom") {
		t.Errorf("night on a 100 baseline = %v, want refused no-headroom", err)
	}
	if ed.calls != 0 {
		t.Errorf("editor ran %d times, want 0", ed.calls)
	}
	f.store.SetOn(f.name, true)
	calls := j.calls
	if _, err := RunNight(context.Background(), f.store, f.name, d); !errors.As(err, &refused) || !strings.Contains(refused.Reason, "no-headroom") {
		t.Errorf("second night = %v, want refused no-headroom", err)
	}
	if j.calls != calls || ed.calls != 0 {
		t.Errorf("second night judged %d times and edited %d times, want neither", j.calls-calls, ed.calls)
	}
}

// Acceptance 12: a role target in a git repo that is not spore lands on
// research/<c> in that repo; the checkout is untouched.
func TestAcceptanceRole12AcceptInOtherGitRepo(t *testing.T) {
	f, root := newRoleCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	_, d := keptNight(t, f, func(b []byte) float64 {
		if bytes.Equal(b, f.original) {
			return 50
		}
		return 80
	})
	if err := Stop(context.Background(), f.store, f.name, "operator", d); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	headBefore := gitIn(t, root, "rev-parse", "HEAD")
	res, err := Accept(f.store, f.name, false)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.Branch != "research/"+f.name || res.Backup != "" {
		t.Errorf("accept result %+v, want branch research/%s and no backup", res, f.name)
	}
	winner := gitIn(t, root, "show", "research/"+f.name+":bootstrap/roles/qa-analyst.md")
	if !strings.Contains(winner, "winner") {
		t.Errorf("branch file = %q, want the winner", winner)
	}
	live, _ := os.ReadFile(f.liveTarget)
	if !bytes.Equal(live, f.original) {
		t.Errorf("live role body changed: %q", live)
	}
	if head := gitIn(t, root, "rev-parse", "HEAD"); head != headBefore {
		t.Errorf("checkout HEAD moved %s -> %s", headBefore, head)
	}
	if st := gitIn(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("checkout is dirty:\n%s", st)
	}
	if br := gitIn(t, root, "branch", "--show-current"); br != "trunk" {
		t.Errorf("checkout branch = %s, want trunk", br)
	}
}
