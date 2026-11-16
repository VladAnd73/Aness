package task

import (
	"strings"
	"testing"

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
