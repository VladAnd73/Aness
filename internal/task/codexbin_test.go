package task

import (
	"strings"
	"testing"

	"github.com/versality/spore/codexpolicy"
	"github.com/versality/spore/internal/task/frontmatter"
)

func TestCodexAgentCommandCodexBin(t *testing.T) {
	t.Setenv(CodexBinEnv, "/opt/codex/bin/codex")
	got, err := CodexAgentCommand(frontmatter.Meta{Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "/opt/codex/bin/codex --dangerously-bypass-approvals-and-sandbox ") {
		t.Errorf("command = %q, want SPORE_CODEX_BIN as argv0", got)
	}
}

// Given a codex task, when its agent command is built (role panes and
// homogeneous workers both use it), then it carries the CLAUDE.md
// project-doc fallback, so a repo without AGENTS.md still gives codex
// its instructions.
func TestCodexAgentCommandCarriesProjectDocFallback(t *testing.T) {
	t.Setenv(CodexBinEnv, "")
	got, err := CodexAgentCommand(frontmatter.Meta{Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "-c " + shellQuote(codexpolicy.ProjectDocFallbackConfig); !strings.Contains(got, want) {
		t.Errorf("command = %q, want it to contain %q", got, want)
	}
}
