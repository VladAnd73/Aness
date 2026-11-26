package install_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	spore "github.com/versality/spore"
)

// adoptedSkills are the three skills adapted from mattpocock/skills.
var adoptedSkills = []string{
	"reviewing-against-the-brief",
	"running-a-harness-retro",
	"building-a-repro-loop",
}

// reservedSkillNames are names a Claude Code built-in or the superpowers
// plugin already owns; a bundled skill with one of them is shadowed.
var reservedSkillNames = map[string]bool{
	"code-review": true, "review": true, "retro": true, "simplify": true,
	"security-review": true, "init": true, "loop": true, "schedule": true,
	"run": true, "diagnosing-bugs": true, "systematic-debugging": true,
	"requesting-code-review": true, "receiving-code-review": true,
}

func adoptedSkillBody(t *testing.T, name string) string {
	t.Helper()
	body, err := fs.ReadFile(spore.BundledSkills, "bootstrap/skills/"+name+"/SKILL.md")
	if err != nil {
		t.Fatalf("bundled skill %s: %v", name, err)
	}
	return string(body)
}

func TestAdoptedSkillsAvoidReservedNamesAndUpstreamSetup(t *testing.T) {
	banned := []*regexp.Regexp{
		regexp.MustCompile(`setup-matt-pocock-skills`),
		regexp.MustCompile(`CODING_STANDARDS\.md`),
		regexp.MustCompile(`(?i)git(hub|lab)\s+(issue|!\d)`),
		regexp.MustCompile(`(?i)issue tracker`),
		regexp.MustCompile(`(?i)originating issue`),
	}
	for _, name := range adoptedSkills {
		if reservedSkillNames[name] {
			t.Errorf("skill name %q collides with a built-in or plugin skill", name)
		}
		body := adoptedSkillBody(t, name)
		for _, re := range banned {
			if loc := re.FindString(body); loc != "" {
				t.Errorf("skill %s references %q", name, loc)
			}
		}
	}
}

func TestReviewSkillUsesBriefAcceptanceScenariosAsSpec(t *testing.T) {
	body := adoptedSkillBody(t, "reviewing-against-the-brief")
	for _, want := range []string{"tasks/<slug>.md", "Acceptance scenarios"} {
		if !strings.Contains(body, want) {
			t.Errorf("review skill does not name %q as the pass-1 spec", want)
		}
	}
}
