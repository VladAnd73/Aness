package fleet

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/versality/spore"
)

func TestParseLoopDefDev(t *testing.T) {
	b, err := fs.ReadFile(spore.BundledLoops, "bootstrap/loops/dev.toml")
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParseLoopDef("embedded:dev", b)
	if err != nil {
		t.Fatalf("ParseLoopDef dev: %v", err)
	}
	if def.Name != "dev" || !def.Summary || !def.GitHubMirror || def.Source != "embedded:dev" {
		t.Errorf("dev header = %q summary=%v mirror=%v source=%q", def.Name, def.Summary, def.GitHubMirror, def.Source)
	}
	if len(def.Steps) != 3 {
		t.Fatalf("dev steps = %d, want 3", len(def.Steps))
	}
	eng := def.Steps[0]
	wantEng := Step{
		Name: "engineer", Kind: "agent", Role: "engineer", Session: "engineer",
		Title: "Engineer", Phase: "engineer-initial", Persist: true,
		Output: "responses/engineer-round-{round}.json", Shape: "engineer-response",
	}
	if !reflect.DeepEqual(eng, wantEng) {
		t.Errorf("engineer step =\n%+v\nwant\n%+v", eng, wantEng)
	}
	for i, slot := range []string{"A", "B"} {
		s := def.Steps[1+i]
		then := "review-B"
		if slot == "B" {
			then = "done"
		}
		want := Step{
			Name: "review-" + slot, Kind: "agent", Role: "reviewer", Session: "reviewer-" + slot,
			Title: "Reviewer " + slot, Label: slot, Phase: "review-" + slot, Slot: slot,
			Output: "reviews/" + slot + "/round-{round}.json", Shape: "verdict", MaxRounds: 3,
			Next: []Transition{
				{On: "request_changes", To: "engineer", Return: true, Phase: "engineer-revise-" + slot},
				{On: "approve", To: then},
			},
		}
		if !reflect.DeepEqual(s, want) {
			t.Errorf("review-%s step =\n%+v\nwant\n%+v", slot, s, want)
		}
	}
	if def.StepIndex("review-B") != 2 || def.StepIndex("missing") != -1 {
		t.Errorf("StepIndex review-B=%d missing=%d", def.StepIndex("review-B"), def.StepIndex("missing"))
	}
}

func TestParseLoopDefExample(t *testing.T) {
	def, err := LoadLoop(t.TempDir(), "example")
	if err != nil {
		t.Fatalf("LoadLoop example: %v", err)
	}
	var names, kinds []string
	for _, s := range def.Steps {
		names = append(names, s.Name)
		kinds = append(kinds, s.Kind)
	}
	if !reflect.DeepEqual(names, []string{"draft", "check", "notify"}) ||
		!reflect.DeepEqual(kinds, []string{"agent", "agent", "action"}) {
		t.Fatalf("example steps = %v kinds = %v", names, kinds)
	}
	check := def.Steps[1]
	if check.Shape != "verdict" || check.MaxRounds != 2 || len(check.Next) == 0 ||
		check.Next[0].To != "draft" || !check.Next[0].Return {
		t.Errorf("check step = %+v", check)
	}
	if def.Steps[2].Action != "notify-coordinator" || def.Steps[2].Params["message"] == "" {
		t.Errorf("notify step = %+v", def.Steps[2])
	}
}

func TestParseLoopDefDefaultsAndFlatActionKeys(t *testing.T) {
	src := `name = "d"

[[steps]]
name = "a"
kind = "agent"
role = "r"
output = "a/round-{round}.json"
shape = "json"
required = ["notes"]

[[steps.done]]
kind = "command"
run = "true"

[[steps.done]]
kind = "command"
run = "true"
timeout = "30s"

[[steps.next]]
to = "b"

[[steps]]
name = "b"
kind = "agent"
role = "r"
output = "b/round-{round}.json"
shape = "verdict"

[[steps.next]]
on = "request_changes"
to = "a"
return = true

[[steps]]
name = "c"
kind = "action"
action = "push-and-open-pr"
label = "QA"
message = "hello"

[steps.params]
extra = "x"
`
	def, err := ParseLoopDef("t.toml", []byte(src))
	if err != nil {
		t.Fatalf("ParseLoopDef: %v", err)
	}
	if len(def.Steps) != 3 || len(def.Steps[0].Done) != 2 || len(def.Steps[1].Next) != 1 {
		t.Fatalf("steps = %+v, want a (2 done), b (1 next), c", def.Steps)
	}
	a := def.Steps[0]
	if a.Title != "a" || a.Phase != "a" || a.Session != "a" {
		t.Errorf("defaults: title=%q phase=%q session=%q, want a", a.Title, a.Phase, a.Session)
	}
	if a.Done[0].TimeoutSeconds != 600 || a.Done[1].TimeoutSeconds != 30 {
		t.Errorf("timeouts = %d, %d, want 600, 30", a.Done[0].TimeoutSeconds, a.Done[1].TimeoutSeconds)
	}
	if got := def.Steps[1].Next[0].Phase; got != "a-revise-b" {
		t.Errorf("return phase = %q, want a-revise-b", got)
	}
	c := def.Steps[2]
	want := map[string]string{"label": "QA", "message": "hello", "extra": "x"}
	if !reflect.DeepEqual(c.Params, want) || c.Label != "QA" {
		t.Errorf("action params = %v label=%q, want %v", c.Params, c.Label, want)
	}
}

func TestValidateRejects(t *testing.T) {
	agent := func(name, extra string) string {
		return "\n[[steps]]\nname = \"" + name + "\"\nkind = \"agent\"\nrole = \"r\"\noutput = \"" + name + "/r-{round}.json\"\nshape = \"verdict\"\n" + extra
	}
	cases := []struct{ name, src, want string }{
		{"unknown kind", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"robot\"\n", `unknown kind "robot"`},
		{"unknown next", "name = \"x\"\n" + agent("a", "[[steps.next]]\nto = \"nowhere\"\n"), `unknown step "nowhere"`},
		{"duplicate step", "name = \"x\"\n" + agent("a", "") + agent("a", ""), `duplicate step "a"`},
		{"needs role", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"agent\"\noutput = \"o\"\nshape = \"json\"\n", `agent step "a" needs role`},
		{"needs output", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"agent\"\nrole = \"r\"\nshape = \"json\"\n", `agent step "a" needs output`},
		{"unknown shape", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"agent\"\nrole = \"r\"\noutput = \"o\"\nshape = \"yaml\"\n", `unknown shape "yaml"`},
		{"unknown action", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"action\"\naction = \"merge-pr\"\n", `unknown action "merge-pr"`},
		{"unknown condition", "name = \"x\"\n" + agent("a", "[[steps.done]]\nkind = \"vibes\"\n"), `unknown condition kind "vibes"`},
		{"command check on action", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"action\"\naction = \"notify-coordinator\"\n[[steps.done]]\nkind = \"command\"\nrun = \"true\"\n", `command condition needs an output`},
		{"command needs run", "name = \"x\"\n" + agent("a", "[[steps.done]]\nkind = \"command\"\n"), `command condition needs run`},
		{"return to later", "name = \"x\"\n" + agent("a", "[[steps.next]]\nto = \"b\"\nreturn = true\n") + agent("b", ""), `return to a later step`},
		{"no steps", "name = \"x\"\n", `no steps`},
		{"unknown key", "name = \"x\"\n" + agent("a", "colour = \"red\"\n"), `unknown key "colour"`},
		{"wrong type", "name = \"x\"\nsummary = \"yes\"\n" + agent("a", ""), `summary`},
		{"verdict field", "name = \"x\"\n" + agent("a", "[[steps.done]]\nkind = \"verdict\"\nfield = \"status\"\nequals = \"ok\"\n"), `verdict condition field "status"`},
		{"output dotdot", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"agent\"\nrole = \"r\"\noutput = \"../a/{round}.json\"\nshape = \"json\"\n", `escapes the task directory`},
		{"output absolute", "name = \"x\"\n[[steps]]\nname = \"a\"\nkind = \"agent\"\nrole = \"r\"\noutput = \"/tmp/a-{round}.json\"\nshape = \"json\"\n", `escapes the task directory`},
		{"file path dotdot", "name = \"x\"\n" + agent("a", "[[steps.done]]\nkind = \"file\"\npath = \"x/../../y\"\n"), `escapes the task directory`},
		{"unknown mode", "name = \"x\"\n" + agent("a", "mode = \"codereview\"\n"), `mode "codereview"`},
		{"unknown slot", "name = \"x\"\n" + agent("a", "slot = \"C\"\n"), `slot "C"`},
		{"unknown effort", "name = \"x\"\n" + agent("a", "effort = \"huge\"\n"), `effort "huge"`},
		{"github_mirror on custom outputs", "name = \"x\"\ngithub_mirror = true\n" + agent("a", ""), `github_mirror`},
		{"github_mirror on renamed dev outputs", "name = \"x\"\ngithub_mirror = true\n" +
			"[[steps]]\nname = \"engineer\"\nkind = \"agent\"\nrole = \"engineer\"\noutput = \"responses/eng-{round}.json\"\nshape = \"engineer-response\"\n" +
			"[[steps]]\nname = \"review-A\"\nkind = \"agent\"\nrole = \"reviewer\"\noutput = \"reviews/A/round-{round}.json\"\nshape = \"verdict\"\n" +
			"[[steps]]\nname = \"review-B\"\nkind = \"agent\"\nrole = \"reviewer\"\noutput = \"reviews/B/round-{round}.json\"\nshape = \"verdict\"\n", `github_mirror`},
		{"verdict condition blocks routing", "name = \"x\"\n" + agent("a", "") + agent("b", "[[steps.done]]\nkind = \"verdict\"\nequals = \"approve\"\n[[steps.next]]\non = \"request_changes\"\nto = \"a\"\n"), `verdict condition equals "approve" blocks next on "request_changes"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseLoopDef("src/x.toml", []byte(c.src))
			if err == nil {
				t.Fatalf("ParseLoopDef accepted:\n%s", c.src)
			}
			if !strings.Contains(err.Error(), c.want) || !strings.HasPrefix(err.Error(), "src/x.toml") {
				t.Errorf("error = %q, want prefix src/x.toml and %q", err, c.want)
			}
		})
	}
}

func TestLoadLoopRoleAliasesDev(t *testing.T) {
	root := t.TempDir()
	role, err := LoadLoop(root, "role")
	if err != nil {
		t.Fatalf("LoadLoop role: %v", err)
	}
	dev, err := LoadLoop(root, "dev")
	if err != nil {
		t.Fatalf("LoadLoop dev: %v", err)
	}
	if role.Name != "dev" || !reflect.DeepEqual(role, dev) {
		t.Errorf("role loop = %+v, want the dev loop %+v", role, dev)
	}
}

func TestLoadLoopUnknownIsErrUnknownLoop(t *testing.T) {
	for _, name := range []string{"nope", "../dev", "dev/x", ""} {
		_, err := LoadLoop(t.TempDir(), name)
		if !errors.Is(err, ErrUnknownLoop) {
			t.Errorf("LoadLoop %q err = %v, want ErrUnknownLoop", name, err)
		}
	}
}

func TestLoadLoopBrokenOverrideFails(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bootstrap", "loops")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing.toml"), filepath.Join(dir, "example.toml")); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLoop(root, "example")
	if err == nil || !strings.Contains(err.Error(), "dangling symlink") {
		t.Errorf("LoadLoop with a dangling override err = %v, want a dangling symlink error", err)
	}
}

func TestLoadLoopBootstrapScriptIsNotAnOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bootstrap"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLoop(root, "example"); err != nil {
		t.Errorf("LoadLoop with a ./bootstrap script: %v, want the embedded loop", err)
	}
}

func TestLoopFixturesParse(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "loops", "*.toml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no loop fixtures: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseLoopDef(p, b)
		broken := strings.HasPrefix(filepath.Base(p), "broken-")
		if broken && err == nil {
			t.Errorf("%s parsed, want an error", p)
		}
		if !broken && err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestParseLoopDefPRFooter(t *testing.T) {
	const steps = "\n[[steps]]\nname = \"a\"\nkind = \"action\"\naction = \"notify-coordinator\"\n"
	if def := mustLoopDef(t, `name = "d"`+steps); !def.PRFooter {
		t.Errorf("pr_footer absent: PRFooter = false, want true")
	}
	if def := mustLoopDef(t, "name = \"d\"\npr_footer = false\n"+steps); def.PRFooter {
		t.Errorf("pr_footer = false: PRFooter = true")
	}
}

// TestGitHubMirrorAcceptsDevOutputsUnderAnyName: the mirror reads the
// dev loop's output paths, so a loop carrying them passes whatever its
// name or step names.
func TestGitHubMirrorAcceptsDevOutputsUnderAnyName(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "bootstrap", "loops", "dev.toml"))
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(string(b), `name = "dev"`, `name = "ship"`, 1)
	src = strings.Replace(src, `name = "review-B"`, `name = "second-look"`, 1)
	src = strings.Replace(src, `to = "review-B"`, `to = "second-look"`, 1)
	if _, err := ParseLoopDef("src/ship.toml", []byte(src)); err != nil {
		t.Fatalf("ParseLoopDef: %v", err)
	}
}

