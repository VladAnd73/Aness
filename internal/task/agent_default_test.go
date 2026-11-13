package task

import (
	"testing"

	"github.com/versality/spore/internal/task/frontmatter"
)

// Bug 7: the Claude Code CLI binary is `claude`; nothing installs a
// `claude-code`, so a spawn without SPORE_AGENT_BINARY died at once.
func TestWorkerAgentCommandDefaultsToClaude(t *testing.T) {
	t.Setenv(AgentBinaryEnv, "")
	for _, agent := range []string{"", "claude", "claude-code"} {
		got, err := workerAgentCommand(frontmatter.Meta{Agent: agent})
		if err != nil {
			t.Fatal(err)
		}
		if got != "claude" {
			t.Errorf("agent %q: spawn command %q, want claude", agent, got)
		}
	}
}

func TestWorkerAgentCommandKeepsTheUnitOverride(t *testing.T) {
	t.Setenv(AgentBinaryEnv, "/usr/local/bin/spore-worker-brief")
	got, err := workerAgentCommand(frontmatter.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/local/bin/spore-worker-brief" {
		t.Fatalf("spawn command %q, want the SPORE_AGENT_BINARY override", got)
	}
}
