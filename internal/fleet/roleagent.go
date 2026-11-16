package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/versality/spore/codexpolicy"
)

// Upstream keeps these helpers in coordinator.go and
// coordinator_config.go; they live here so the role-loop port leaves
// our copies of those files untouched.

func bodyShellCommand(cmdWithBody, cmdBare, rolePath string) string {
	return fmt.Sprintf(
		`if [ -r %[1]s ] && [ -s %[1]s ]; then exec %[2]s "$(cat %[1]s)"; else exec %[3]s; fi`,
		shellSingleQuote(rolePath), cmdWithBody, cmdBare,
	)
}

// agentInvocations returns the agent command for a run with a prompt
// body and for a bare run. Role bodies open with `---` frontmatter, so
// the body run ends in `--` for both claude and codex.
func agentInvocations(agent string) (withBody, bare string) {
	if isClaudeAgent(agent) {
		return agent + " --dangerously-skip-permissions --", agent + " --dangerously-skip-permissions"
	}
	if isCodexAgent(agent) {
		codex := agent + " -c " + shellSingleQuote(codexpolicy.ProjectDocFallbackConfig)
		return codex + " --", codex
	}
	return agent, agent
}

// isClaudeAgent reports whether the agent string resolves to the
// claude CLI. The agent value can carry trailing args (sleep 30,
// codex exec --model ...), so we tokenise and check field 0's
// basename.
func isClaudeAgent(agent string) bool {
	fields := strings.Fields(agent)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	return base == "claude" || base == "claude-code"
}

func isCodexAgent(agent string) bool {
	fields := strings.Fields(agent)
	return len(fields) > 0 && filepath.Base(fields[0]) == "codex"
}

// writePrivateFile replaces path via a temp file and rename, mode 0600
// in a 0700 directory.
func writePrivateFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
