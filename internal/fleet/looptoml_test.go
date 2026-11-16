package fleet

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoopTOMLSubset(t *testing.T) {
	src := `# leading comment
name = "demo" # trailing comment
summary = true
github_mirror = false

[[steps]]
name = "a"
kind = "agent"
run = "echo \"hi\" # not a comment\\n"
max_rounds = 3
skills = ["x", "y",]
empty = []

[steps.params]
label = "L"

[[steps.done]]
kind = "command"
timeout = 600

[[steps.next]]
on = "approve"
to = "done"
return = false

[[steps]]
name = "b"
`
	doc, err := parseLoopTOML([]byte(src))
	if err != nil {
		t.Fatalf("parseLoopTOML: %v", err)
	}
	if got := doc.top["name"].v; got != "demo" {
		t.Errorf("name = %#v, want demo", got)
	}
	if got := doc.top["summary"].v; got != true {
		t.Errorf("summary = %#v, want true", got)
	}
	if got := doc.top["github_mirror"].v; got != false {
		t.Errorf("github_mirror = %#v, want false", got)
	}
	if len(doc.steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(doc.steps))
	}
	a := doc.steps[0]
	if got := a.keys["run"].v; got != `echo "hi" # not a comment\n` {
		t.Errorf("run = %#v", got)
	}
	if got := a.keys["max_rounds"].v; got != 3 {
		t.Errorf("max_rounds = %#v, want 3", got)
	}
	if got := a.keys["skills"].v; !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Errorf("skills = %#v", got)
	}
	if got := a.keys["empty"].v; !reflect.DeepEqual(got, []string{}) {
		t.Errorf("empty = %#v", got)
	}
	if got := a.params["label"].v; got != "L" {
		t.Errorf("params.label = %#v", got)
	}
	if len(a.done) != 1 || a.done[0]["timeout"].v != 600 {
		t.Errorf("done = %#v", a.done)
	}
	if len(a.next) != 1 || a.next[0]["to"].v != "done" || a.next[0]["return"].v != false {
		t.Errorf("next = %#v", a.next)
	}
	if got := doc.steps[1].keys["name"].v; got != "b" {
		t.Errorf("second step name = %#v", got)
	}
}

func TestLoopTOMLRejects(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"inline table", "name = \"x\"\nopts = { a = 1 }\n", "line 2"},
		{"multi-line basic string", "name = \"x\"\n\n[[steps]]\nrun = \"\"\"\nhi\n\"\"\"\n", "line 4"},
		{"literal string", "name = 'x'\n", "line 1"},
		{"multi-line array", "skills = [\n\"a\"]\n", "line 1"},
		{"unknown table", "[fleet]\n", "line 1"},
		{"params before steps", "[steps.params]\n", "line 1"},
		{"done before steps", "[[steps.done]]\n", "line 1"},
		{"dotted key", "a.b = 1\n", "line 1"},
		{"duplicate key", "name = \"a\"\nname = \"b\"\n", "line 2"},
		{"float", "x = 1.5\n", "line 1"},
		{"unterminated string", "x = \"abc\n", "line 1"},
		{"bad escape", "x = \"a\\qb\"\n", "line 1"},
		{"trailing garbage", "x = \"a\" b\n", "line 1"},
		{"missing value", "x =\n", "line 1"},
		{"non-string array", "x = [1, 2]\n", "line 1"},
		{"bare word value", "x = yes\n", "line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseLoopTOML([]byte(c.src))
			if err == nil {
				t.Fatalf("parseLoopTOML accepted %q", c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to contain %q", err, c.want)
			}
		})
	}
}
