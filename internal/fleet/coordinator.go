package fleet

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	spore "github.com/versality/spore"
	"github.com/versality/spore/internal/agentenv"
	"github.com/versality/spore/internal/hooks"
	"github.com/versality/spore/internal/task"
)

// coordinatorSpawnSettleDelay is the wait between `tmux new-session -d`
// and the post-spawn `has-session` check. tmux returns success the
// moment its server has registered the session, before the inner shell
// has had a chance to `exec` the agent. A typo in the agent binary
// (e.g. `claude-code` when only `claude` is installed) causes the
// child to die within a few ms, after which the session is gone. The
// settle window catches that case so EnsureCoordinator surfaces a real
// error instead of lying with "spawned".
//
// Sized for slow exec paths (shim that resolves shared+project role
// files via `cat`, then forks again to exec the agent). 150ms tripped
// on a host where the user-service PATH was missing bashInteractive,
// causing `#!/usr/bin/env bash` to die before claude was ever exec'd;
// fixing the service PATH made that the dominant failure mode, but
// the wider window costs the reconciler nothing on the happy path and
// shrugs off filesystem-cold reads on the role files.
const coordinatorSpawnSettleDelay = 300 * time.Millisecond

// CoordinatorSlug is the reserved session slug for the singleton
// coordinator agent. Workers cannot use it; the fleet reconciler
// manages this session out-of-band from the per-task queue.
const CoordinatorSlug = "coordinator"

// CoordinatorRoleEnv overrides the role file path the reconciler
// hands to the coordinator session. Empty falls back to
// <projectRoot>/bootstrap/coordinator/role.md.
const CoordinatorRoleEnv = "SPORE_COORDINATOR_ROLE_FILE"

// CoordinatorAgentEnv selects the binary the coordinator session
// execs. Read before SPORE_AGENT_BINARY so operators can run a
// different agent (or a greet-and-shell wrapper) for the singleton
// coordinator without affecting per-task workers.
const CoordinatorAgentEnv = "SPORE_COORDINATOR_AGENT"

// CoordinatorSessionName returns the tmux session for the singleton
// coordinator: "spore/<project>/coordinator", parallel to worker
// session names. Resolves the project name via task.ProjectName so
// invocations from a worktree cwd still target the main repo session
// instead of forking a stray "spore/<slug>/coordinator".
func CoordinatorSessionName(projectRoot string) string {
	name, err := task.ProjectName(projectRoot)
	if err != nil || name == "" {
		name = filepath.Base(projectRoot)
	}
	return fmt.Sprintf("spore/%s/%s", name, CoordinatorSlug)
}

// CoordinatorRolePath returns the override path from
// SPORE_COORDINATOR_ROLE_FILE if set, else the [coordinator].brief
// entry in spore.toml (resolved against projectRoot when relative),
// else the in-tree default at <projectRoot>/bootstrap/coordinator/role.md.
func CoordinatorRolePath(projectRoot string) string {
	if p := os.Getenv(CoordinatorRoleEnv); p != "" {
		return p
	}
	if cfg, err := LoadCoordinatorConfig(projectRoot); err == nil && cfg.Brief != "" {
		if filepath.IsAbs(cfg.Brief) {
			return cfg.Brief
		}
		return filepath.Join(projectRoot, cfg.Brief)
	}
	return defaultRolePath(projectRoot)
}

func defaultRolePath(projectRoot string) string {
	return filepath.Join(projectRoot, "bootstrap", "coordinator", "role.md")
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// writeBundledRole drops the role embedded in this binary into dir so
// a project without its own role file still boots a seeded
// coordinator. Rewritten on every spawn so a spore upgrade reaches the
// next coordinator without a manual copy.
func writeBundledRole(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "bundled-role.md")
	if err := os.WriteFile(path, spore.BundledCoordinatorRole, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// EnsureCoordinator spawns the coordinator tmux session for projectRoot
// when it is not already alive. Idempotent: a live session is left
// alone. The session runs in projectRoot itself (no worktree) with
// SPORE_TASK_SLUG=coordinator and SPORE_COORDINATOR_ROLE=<path> in the
// session env. The session's command is a small shell snippet that
// passes the role file's contents as the agent's first positional
// arg when the file is readable and non-empty (so a default
// claude-code agent boots with the role as its first user message).
// When the default in-tree role file is missing, the role embedded in
// the binary is written out and SPORE_COORDINATOR_ROLE points at it,
// but only a claude or codex agent also gets it as an argument: a
// custom launcher saw no arguments here before the fallback existed and
// may reject them. An explicit override that points at a missing file
// spawns the agent bare. Returns the session name and whether a spawn
// actually happened.
func EnsureCoordinator(projectRoot string) (string, bool, error) {
	session := CoordinatorSessionName(projectRoot)
	if hasSession(session) {
		return session, false, nil
	}

	tomlCfg, _ := LoadCoordinatorConfig(projectRoot)
	if pat := tomlCfg.ExternalSessionPattern; pat != "" {
		if name, ok := externalCoordinatorSession(pat); ok {
			return name, false, nil
		}
	}
	agent := coordinatorAgent(tomlCfg)
	rolePath := CoordinatorRolePath(projectRoot)
	project, err := task.ProjectName(projectRoot)
	if err != nil {
		return "", false, err
	}
	inbox, err := task.CoordinatorInboxDirForProject(projectRoot)
	if err != nil {
		return "", false, err
	}
	coordinatorState, err := task.CoordinatorStateDir()
	if err != nil {
		return "", false, err
	}

	projectEnv, envMsgs, err := agentenv.Load(project)
	for _, m := range envMsgs {
		fmt.Fprintf(os.Stderr, "agent.env: %s\n", m)
	}
	if err != nil {
		return "", false, err
	}

	passRole := true
	if rolePath == defaultRolePath(projectRoot) && !nonEmptyFile(rolePath) {
		rolePath, err = writeBundledRole(filepath.Join(coordinatorState, project))
		if err != nil {
			return "", false, err
		}
		passRole = agentTakesPrompt(agent)
	}

	cmd := coordinatorShellCommand(agent, rolePath, passRole)
	args := []string{
		"-d",
		"-s", session,
		"-c", projectRoot,
		"-e", "SPORE_TASK_SLUG=" + CoordinatorSlug,
		"-e", "SPORE_COORDINATOR_ROLE=" + rolePath,
		"-e", "SPORE_PROJECT_ROOT=" + projectRoot,
		"-e", "WT_PROJECT=" + project,
		"-e", "SPORE_TASK_INBOX=" + inbox,
		"-e", "SPORE_COORDINATOR_STATE_DIR=" + coordinatorState,
	}
	if v := coordinatorProvider(tomlCfg); v != "" {
		args = append(args, "-e", "SPORE_COORDINATOR_PROVIDER="+v)
	}
	if v := coordinatorModel(tomlCfg); v != "" {
		args = append(args, "-e", "SPORE_COORDINATOR_MODEL="+v)
	}
	args = append(args, agentenv.TmuxArgs(projectEnv)...)
	out, err := task.TmuxNewSession(args, cmd).CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("tmux new-session: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if pane := task.SessionPaneID(session); pane != "" {
		if err := hooks.RecordInboxOwner(inbox, pane); err != nil {
			fmt.Fprintf(os.Stderr, "spore: record coordinator inbox owner: %v\n", err)
		}
	}
	// tmux registers the session before the inner shell execs the
	// agent. Wait briefly, then confirm the session survived; an
	// agent binary that fails to exec tears the session down within
	// a few ms.
	time.Sleep(coordinatorSpawnSettleDelay)
	if !hasSession(session) {
		return "", false, fmt.Errorf(
			"coordinator session %s died on spawn (agent=%q): the inner exec failed before the session could settle. Check that the agent binary is on PATH",
			session, agent,
		)
	}
	return session, true, nil
}

// coordinatorAgent picks the binary the coordinator session execs.
// Precedence: SPORE_COORDINATOR_AGENT (lets operators run a different
// agent for the singleton coordinator) > SPORE_AGENT_BINARY (the same
// var workers honour) > spore.toml [coordinator].driver mapped to a
// known binary > "claude" (the kernel default).
func coordinatorAgent(cfg CoordinatorConfig) string {
	if a := os.Getenv(CoordinatorAgentEnv); a != "" {
		return a
	}
	if a := os.Getenv(task.AgentBinaryEnv); a != "" {
		return a
	}
	if a := driverToBinary(cfg.Driver); a != "" {
		return a
	}
	return "claude"
}

// coordinatorProvider returns the provider name for launcher scripts
// that dispatch on $SPORE_COORDINATOR_PROVIDER (e.g. the bundled
// spore-coordinator-launch.sh). Env wins over the spore.toml driver.
// Empty when nothing is configured: the session env stays clean.
func coordinatorProvider(cfg CoordinatorConfig) string {
	if v := os.Getenv("SPORE_COORDINATOR_PROVIDER"); v != "" {
		return v
	}
	return cfg.Driver
}

// coordinatorModel returns the model identifier injected into the
// session env. Env wins over the spore.toml model. Empty when neither
// source set it.
func coordinatorModel(cfg CoordinatorConfig) string {
	if v := os.Getenv("SPORE_COORDINATOR_MODEL"); v != "" {
		return v
	}
	return cfg.Model
}

// driverToBinary maps a friendly driver name to the binary to exec.
// "claude" -> "claude" (the Anthropic CLI binary name; the package is
// often called "claude-code" but its bin/ entry is "claude"), "codex"
// -> "codex". Unknown values pass through verbatim so a project can
// wire a launcher script by name. Empty input returns empty so the
// caller falls through to the next precedence level.
func driverToBinary(driver string) string {
	switch driver {
	case "":
		return ""
	case "claude":
		return "claude"
	default:
		return driver
	}
}

// coordinatorShellCommand builds the shell snippet tmux runs for the
// coordinator session. tmux invokes its operator shell to parse this
// string; the agent token is intentionally left unquoted so callers
// can pass space-bearing values (e.g. SPORE_AGENT_BINARY="sleep 30")
// the same way worker spawn does.
func coordinatorShellCommand(agent, rolePath string, passRole bool) string {
	if !passRole {
		return "exec " + agent
	}
	q := shellSingleQuote(rolePath)
	return fmt.Sprintf(
		`if [ -r %[1]s ] && [ -s %[1]s ]; then exec %[2]s "$(cat %[1]s)"; else exec %[2]s; fi`,
		q, agent,
	)
}

// agentTakesPrompt reports whether agent's binary is one spore knows
// takes a positional first prompt. The agent string may carry flags.
func agentTakesPrompt(agent string) bool {
	fields := strings.Fields(agent)
	if len(fields) == 0 {
		return false
	}
	switch filepath.Base(fields[0]) {
	case "claude", "codex":
		return true
	}
	return false
}

// shellSingleQuote returns s wrapped in single quotes, with embedded
// single quotes escaped, suitable for splicing into a shell-command
// string passed through tmux.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ReapCoordinator kills the coordinator tmux session for projectRoot.
// Idempotent: a missing session is not an error. Returns whether a
// kill was attempted.
func ReapCoordinator(projectRoot string) bool {
	session := CoordinatorSessionName(projectRoot)
	if !hasSession(session) {
		return false
	}
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	return true
}

// CoordinatorAlive reports whether the coordinator tmux session for
// projectRoot is currently up. A matching external_session_pattern
// counts as alive so the wait poll does not spin against an
// operator-managed coordinator at a non-kernel session name.
func CoordinatorAlive(projectRoot string) bool {
	if hasSession(CoordinatorSessionName(projectRoot)) {
		return true
	}
	if cfg, err := LoadCoordinatorConfig(projectRoot); err == nil && cfg.ExternalSessionPattern != "" {
		if _, ok := externalCoordinatorSession(cfg.ExternalSessionPattern); ok {
			return true
		}
	}
	return false
}

// externalCoordinatorSession returns the first tmux session name matching
// pattern (RE2). Used by EnsureCoordinator and CoordinatorAlive to defer
// to an operator-side coordinator running under a non-kernel session
// name. A malformed regex or an unreachable tmux server (no server
// running, exec failure) returns no match so the caller falls back to
// kernel-managed lifecycle instead of failing the reconcile pass.
func externalCoordinatorSession(pattern string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false
	}
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return "", false
	}
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" {
			continue
		}
		if re.MatchString(name) {
			return name, true
		}
	}
	return "", false
}

func hasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}
