package task

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/versality/spore/codexpolicy"
	"github.com/versality/spore/evidence"
	"github.com/versality/spore/internal/agentenv"
	"github.com/versality/spore/internal/hooks"
	"github.com/versality/spore/internal/matter"
	"github.com/versality/spore/internal/task/frontmatter"
)

// EvidenceWarnOnlyEnv forces the evidence done-gate into warn-only
// mode regardless of the soak window. The soak window already gates
// warn-only behavior for the first 7 days after evidence.ContractStart;
// the env var stays as a permanent rollback override per the brief.
const EvidenceWarnOnlyEnv = "SPORE_EVIDENCE_WARN_ONLY"

// AgentBinaryEnv is the env var used to override the binary spawned in
// the per-task tmux session. Defaults to defaultAgentBinary when unset.
const AgentBinaryEnv = "SPORE_AGENT_BINARY"

const defaultAgentBinary = "claude"

// CodexModelEnv optionally pins the model for `agent: codex` task
// launches. Empty lets the codex CLI use its own default.
const CodexModelEnv = "SPORE_CODEX_MODEL"

// Start flips status to active and (when starting from draft) creates
// the worktree and wt/<slug> branch under <projectRoot>/.worktrees/.
// In every case it spawns a detached tmux session named
// "spore/<project>/<slug>" running ${SPORE_AGENT_BINARY:-claude}
// in the worktree, with SPORE_TASK_SLUG=<slug> in the session env.
// Returns the tmux session name on success.
func Start(tasksDir, slug string) (string, error) {
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m, body, err := frontmatter.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	prev := m.Status
	switch prev {
	case "draft", "paused", "blocked":
	case "active":
		return "", fmt.Errorf("task %s: already active", slug)
	case "done":
		return "", fmt.Errorf("task %s: already done", slug)
	default:
		return "", fmt.Errorf("task %s: unexpected status %q", slug, prev)
	}
	m.Status = "active"
	if err := os.WriteFile(path, frontmatter.Write(m, body), 0o644); err != nil {
		return "", err
	}

	projectRoot, err := projectRootFromTasksDir(tasksDir)
	if err != nil {
		return "", err
	}
	session := taskTmuxSession(tasksDir, projectRoot, slug)
	// Pause leaves the session alive for the operator; Start
	// replaces it so a resume gets a fresh agent and new-session
	// does not collide on the name.
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	out, err := ensureSession(tasksDir, slug)
	if err != nil {
		return "", err
	}
	notifyMatterSpawn(projectRoot, slug, m, os.Stderr)
	return out, nil
}

// Ensure makes sure the wt/<slug> branch, worktree, and tmux session
// for slug exist. Idempotent: missing pieces get created, present
// ones are left alone. Status is not touched. Used by the fleet
// reconciler to bring an active task into the running state without
// flipping its status.
func Ensure(tasksDir, slug string) (string, error) {
	return ensureSession(tasksDir, slug)
}

// Reap kills every tmux session matching slug for the project (the
// frontmatter-recorded one plus any duplicates that drifted during
// spawn or got minted with a different tier tag). Status, worktree,
// and branch are left untouched. Used by the fleet reconciler when a
// task leaves active.
func Reap(tasksDir, projectRoot, slug string) error {
	killAllSlugSessions(tasksDir, projectRoot, slug)
	return nil
}

// SpawnedSlugs lists slugs of every tmux session that matches the
// "spore/<project>/<slug>" pattern. Returns an empty slice (and a
// nil error) when no tmux server is running.
func SpawnedSlugs(projectRoot string) ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		// tmux exits non-zero with "no server running" or no
		// sessions; treat both as empty.
		return nil, nil
	}
	prefix := tmuxSessionPrefix(projectRoot)
	var slugs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}
		slugs = append(slugs, strings.TrimPrefix(line, prefix))
	}
	sort.Strings(slugs)
	return slugs, nil
}

// Pause flips an active task to paused. The worktree is left in
// place; the tmux session is reaped only if it has been idle past
// IdleReapThreshold (model exited, pane sitting at an empty prompt).
// A mid-tool-call rower stays alive so the operator can attach and
// finish a thought. Refuses when the inbox has unread messages.
func Pause(tasksDir, slug string) error {
	if err := inboxGate(slug); err != nil {
		return err
	}
	if err := flipStatus(tasksDir, slug, "active", "paused"); err != nil {
		return err
	}
	reapIdleSlugSessions(tasksDir, slug)
	return nil
}

// Block flips an active task to blocked. Same idle-gated reap as
// Pause: the worktree stays, the session is killed only if idle past
// IdleReapThreshold. Refuses when the inbox has unread messages.
func Block(tasksDir, slug string) error {
	if err := inboxGate(slug); err != nil {
		return err
	}
	if err := flipStatus(tasksDir, slug, "active", "blocked"); err != nil {
		return err
	}
	reapIdleSlugSessions(tasksDir, slug)
	return nil
}

// Verify reads tasks/<slug>.md and runs the structural evidence
// verifier. Returns the verdict plus diagnostic lines. Used by
// `spore task verify` so the operator can preview the gate's decision
// without touching status.
func Verify(tasksDir, slug string) (evidence.Verdict, []string, error) {
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	m, body, err := frontmatter.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", path, err)
	}
	verdict, diags := evidence.Verify(metaToAny(m), string(body))
	return verdict, diags, nil
}

// Done flips a task to done and best-effort cleans up the tmux
// session, worktree, and wt/<slug> branch. Errors from cleanup are
// surfaced to stderr so a broken chain is visible; the status flip
// remains the source of truth. Calling Done on an already-done task
// is a no-op.
//
// When force is true, the inbox-drain and unmerged-commit gates are
// bypassed; the evidence gate still runs (it has its own soak/env
// override).
func Done(tasksDir, slug string, force bool) error {
	// Resolve to canonical main-repo paths first. When a rover calls
	// `spore task done` against its own slug, the CLI's hardcoded
	// "tasks" resolves against the rover's worktree cwd, so tasksDir
	// arrives as <worktree>/tasks/. projectRootFromTasksDir uses
	// --git-common-dir to hop back to the main repo; re-derive
	// tasksDir from that so the status flip lands on the
	// source-of-truth file (the one `spore task ls` reads) and the
	// worktree-remove targets the real `<main>/.worktrees/<slug>`.
	projectRoot, err := projectRootFromTasksDir(tasksDir)
	if err != nil {
		return err
	}
	tasksDir = filepath.Join(projectRoot, filepath.Base(tasksDir))

	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m, body, err := frontmatter.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Status == "done" {
		return nil
	}

	if !force {
		if err := inboxGate(slug); err != nil {
			return err
		}
	}

	branch := "wt/" + slug
	unmerged, err := UnmergedCommits(projectRoot, branch)
	if err != nil {
		return err
	}
	if unmerged > 0 {
		if force {
			fmt.Fprintf(os.Stderr, "spore task done %s: --force: discarding %d unmerged commit(s) on %s\n", slug, unmerged, branch)
		} else {
			return fmt.Errorf("done refused for %s: branch %q has %d unmerged commit(s); run 'spore task merge %s' first, or 'spore task done %s --force' to discard", slug, branch, unmerged, slug, slug)
		}
	}

	if err := evidenceGate(slug, m, body, os.Stderr); err != nil {
		return err
	}

	m.Status = "done"
	if err := os.WriteFile(path, frontmatter.Write(m, body), 0o644); err != nil {
		return err
	}

	notifyMatterDone(projectRoot, slug, m, os.Stderr)

	worktree := filepath.Join(projectRoot, ".worktrees", slug)

	killAllSlugSessions(tasksDir, projectRoot, slug)
	if _, statErr := os.Stat(worktree); statErr == nil {
		if out, err := gitCmd(projectRoot, "worktree", "remove", "--force", worktree).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "spore task done %s: git worktree remove %s: %v: %s\n",
				slug, worktree, err, strings.TrimSpace(string(out)))
		}
	}
	if branchExists(projectRoot, branch) {
		if out, err := gitCmd(projectRoot, "branch", "-D", branch).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "spore task done %s: git branch -D %s: %v: %s\n",
				slug, branch, err, strings.TrimSpace(string(out)))
		}
	}
	if dir, err := slugServicesDir(slug); err != nil {
		fmt.Fprintf(os.Stderr, "spore task done %s: services dir resolve: %v\n", slug, err)
	} else if _, statErr := os.Stat(dir); statErr == nil {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			fmt.Fprintf(os.Stderr, "spore task done %s: rm -r %s: %v\n", slug, dir, rmErr)
		}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "spore task done %s: rm %s: %v\n", slug, path, err)
	}
	return nil
}

// notifyMatterDone fires OnDone on the matter named in the task's
// frontmatter (Extra["matter"]). No-op when the key is absent or the
// adapter isn't configured for this project. Errors land on warnOut;
// the status flip remains the source of truth.
func notifyMatterDone(projectRoot, slug string, m frontmatter.Meta, warnOut io.Writer) {
	name := m.Extra[matter.MatterKey]
	if name == "" {
		return
	}
	configs, err := matter.LoadFromProject(projectRoot)
	if err != nil {
		fmt.Fprintf(warnOut, "spore task done %s: matter load: %v\n", slug, err)
		return
	}
	var cfg *matter.Config
	for i := range configs {
		if configs[i].Name == name && configs[i].Enabled {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil {
		return
	}
	matters, err := matter.FromConfig([]matter.Config{*cfg})
	if err != nil {
		fmt.Fprintf(warnOut, "spore task done %s: matter %s: %v\n", slug, name, err)
		return
	}
	if len(matters) == 0 {
		return
	}
	if err := matters[0].OnDone(context.Background(), slug, copyExtra(m.Extra)); err != nil {
		fmt.Fprintf(warnOut, "spore task done %s: matter %s OnDone: %v\n", slug, name, err)
	}
}

// NotifyMatterSpawn fires OnSpawn on the matter named in the task's
// frontmatter (Extra["matter"]) - the rover-claim signal. The fleet
// reconciler calls it after task.Ensure brings up a new tmux
// session; lifecycle.Start calls the package-private notifyMatterSpawn
// directly after creating its session. No-op when the key is absent
// or the adapter isn't configured for this project. Errors land on
// warnOut; the spawn itself is the source of truth.
func NotifyMatterSpawn(projectRoot, tasksDir, slug string, warnOut io.Writer) {
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(warnOut, "spore notify-matter-spawn %s: read brief: %v\n", slug, err)
		return
	}
	m, _, err := frontmatter.Parse(raw)
	if err != nil {
		fmt.Fprintf(warnOut, "spore notify-matter-spawn %s: parse brief: %v\n", slug, err)
		return
	}
	notifyMatterSpawn(projectRoot, slug, m, warnOut)
}

// notifyMatterSpawn is the inner shared path used when the caller
// already has the parsed Meta and can skip the re-read.
func notifyMatterSpawn(projectRoot, slug string, m frontmatter.Meta, warnOut io.Writer) {
	name := m.Extra[matter.MatterKey]
	if name == "" {
		return
	}
	configs, err := matter.LoadFromProject(projectRoot)
	if err != nil {
		fmt.Fprintf(warnOut, "spore notify-matter-spawn %s: matter load: %v\n", slug, err)
		return
	}
	var cfg *matter.Config
	for i := range configs {
		if configs[i].Name == name && configs[i].Enabled {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil {
		return
	}
	matters, err := matter.FromConfig([]matter.Config{*cfg})
	if err != nil {
		fmt.Fprintf(warnOut, "spore notify-matter-spawn %s: matter %s: %v\n", slug, name, err)
		return
	}
	if len(matters) == 0 {
		return
	}
	if err := matters[0].OnSpawn(context.Background(), slug, copyExtra(m.Extra)); err != nil {
		fmt.Fprintf(warnOut, "spore notify-matter-spawn %s: matter %s OnSpawn: %v\n", slug, name, err)
	}
}

func copyExtra(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// evidenceGate runs the structural evidence verifier on the task body
// and refuses the done flip when the verdict blocks. Pre-contract
// tasks (no evidence_required declared) are skipped silently. During
// the soak window or when SPORE_EVIDENCE_WARN_ONLY=1 is set, blocking
// verdicts are reduced to a stderr warning.
func evidenceGate(slug string, m frontmatter.Meta, body []byte, warnOut *os.File) error {
	meta := metaToAny(m)
	if len(evidence.Required(meta)) == 0 {
		return nil
	}
	verdict, diags := evidence.Verify(meta, string(body))
	if !evidence.Blocks(verdict) {
		return nil
	}
	msg := fmt.Sprintf("evidence verdict: %s", verdict)
	for _, d := range diags {
		msg += "\n  " + d
	}
	warnOnly := os.Getenv(EvidenceWarnOnlyEnv) == "1" || evidence.InSoakWindow(time.Now())
	if !warnOnly {
		return fmt.Errorf("done refused for %s: %s", slug, msg)
	}
	if warnOut != nil {
		fmt.Fprintf(warnOut, "spore task done %s: warn-only: %s\n", slug, msg)
	}
	return nil
}

// inboxGate refuses the status flip when the slug's inbox has unread
// *.json files. Returns nil when the inbox is empty or missing.
func inboxGate(slug string) error {
	n, dir, err := CountUnreadInbox(slug)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%d unread inbox message(s) at %s; read each, then mv to inbox/read/", n, dir)
	}
	return nil
}

// metaToAny lifts frontmatter.Meta into the map[string]any shape
// evidence.Required and evidence.Verify accept. Spore's parser only
// stores strings, so this is just a key-by-key copy.
func metaToAny(m frontmatter.Meta) map[string]any {
	out := map[string]any{}
	if m.Status != "" {
		out["status"] = m.Status
	}
	if m.Slug != "" {
		out["slug"] = m.Slug
	}
	if m.Title != "" {
		out["title"] = m.Title
	}
	if m.Created != "" {
		out["created"] = m.Created
	}
	if m.Project != "" {
		out["project"] = m.Project
	}
	if m.Host != "" {
		out["host"] = m.Host
	}
	if m.Agent != "" {
		out["agent"] = m.Agent
	}
	for k, v := range m.Extra {
		out[k] = v
	}
	return out
}

// ensureSession is the shared idempotent path for Start and Ensure.
// It creates the worktree + branch when missing (re-attaching to an
// existing branch when the worktree was removed) and (re)spawns the
// tmux session when not already alive.
func ensureSession(tasksDir, slug string) (string, error) {
	projectRoot, err := projectRootFromTasksDir(tasksDir)
	if err != nil {
		return "", err
	}
	meta, err := readTaskMeta(tasksDir, slug)
	if err != nil {
		return "", err
	}
	worktree := filepath.Join(projectRoot, ".worktrees", slug)
	branch := "wt/" + slug

	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		args := []string{"worktree", "add", worktree}
		if branchExists(projectRoot, branch) {
			args = append(args, branch)
		} else {
			args = append(args, "-b", branch)
		}
		out, err := gitCmd(projectRoot, args...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git worktree add: %v: %s", err, strings.TrimSpace(string(out)))
		}
		// Copy the brief into the new worktree so headless workers
		// can read it. The worktree forks from the source branch's
		// HEAD which often does not yet include this brief: Start
		// rewrites it just before this call (status flip), and the
		// operator may not have committed it on the source branch
		// either. Soft-fails on a missing source brief; the worker
		// falls back to interactive mode there.
		if err := copyBriefToWorktree(tasksDir, worktree, slug); err != nil {
			return "", fmt.Errorf("copy brief: %w", err)
		}
	}

	if external := meta.Session; external != "" && hasSession(external) {
		return external, nil
	}
	session := tmuxSessionName(projectRoot, slug)
	if hasSession(session) {
		return session, nil
	}
	agent, err := workerSpawnCommand(projectRoot, meta)
	if err != nil {
		return "", err
	}
	project, err := ProjectName(projectRoot)
	if err != nil {
		return "", err
	}
	inbox, err := InboxDirForProject(projectRoot, slug)
	if err != nil {
		return "", err
	}
	coordinatorState, err := CoordinatorStateDir()
	if err != nil {
		return "", err
	}
	projectEnv, envMsgs, err := agentenv.Load(project)
	for _, m := range envMsgs {
		fmt.Fprintf(os.Stderr, "agent.env: %s\n", m)
	}
	if err != nil {
		return "", err
	}

	args := []string{
		"-d",
		"-s", session,
		"-c", worktree,
		"-e", "SPORE_TASK_SLUG=" + slug,
		"-e", "SPORE_PROJECT_ROOT=" + projectRoot,
		"-e", "WT_PROJECT=" + project,
		"-e", "SPORE_TASK_INBOX=" + inbox,
		"-e", "SPORE_COORDINATOR_STATE_DIR=" + coordinatorState,
	}
	args = append(args, agentenv.TmuxArgs(projectEnv)...)
	out, err := TmuxNewSession(args, agent).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux new-session: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if pane := SessionPaneID(session); pane != "" {
		if err := hooks.RecordInboxOwner(inbox, pane); err != nil {
			fmt.Fprintf(os.Stderr, "spore: record inbox owner for %s: %v\n", slug, err)
		}
	}
	// tmux registers the session before its shell execs the agent, so
	// new-session succeeds even for a binary that does not exist.
	time.Sleep(SpawnSettleDelay)
	if SessionPaneID(session) == "" {
		return "", fmt.Errorf("worker session %s died on spawn (agent=%q): check %s and that the binary is on PATH", session, agent, AgentBinaryEnv)
	}
	return session, nil
}

// SpawnSettleDelay is the wait between tmux new-session and the check
// that the agent is still running. Same window as the coordinator's.
const SpawnSettleDelay = 300 * time.Millisecond

// SessionPaneID returns the active pane of the exactly-named session, or "" when
// no such session exists. A bare -t name would also match a session that
// merely starts with it.
func SessionPaneID(session string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session+":", "#{pane_id}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readTaskMeta(tasksDir, slug string) (frontmatter.Meta, error) {
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return frontmatter.Meta{}, nil
		}
		return frontmatter.Meta{}, err
	}
	m, _, err := frontmatter.Parse(raw)
	if err != nil {
		return frontmatter.Meta{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func workerAgentCommand(m frontmatter.Meta) (string, error) {
	if override := os.Getenv(AgentBinaryEnv); override != "" {
		return override, nil
	}
	agent := m.Agent
	if agent == "" || agent == "claude" || agent == "claude-code" {
		return defaultAgentBinary, nil
	}
	if agent != "codex" {
		return agent, nil
	}

	effort, err := codexpolicy.EffortForTask(m.Extra["effort"], m.Extra["complexity"])
	if err != nil {
		return "", err
	}
	model := m.Extra["model"]
	if model == "" {
		model = os.Getenv(CodexModelEnv)
	}
	return shellJoin(codexpolicy.InteractiveArgs(model, effort)), nil
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool { return !isShellBareChar(r) }) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellBareChar(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '='
}

func flipStatus(tasksDir, slug, from, to string) error {
	path := filepath.Join(tasksDir, slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m, body, err := frontmatter.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Status != from {
		return fmt.Errorf("task %s: status %q (want %q)", slug, m.Status, from)
	}
	m.Status = to
	return os.WriteFile(path, frontmatter.Write(m, body), 0o644)
}

// projectRootFromTasksDir resolves tasksDir to the main repo root.
// When tasksDir lives inside a linked worktree (the common case for a
// rover invoking `spore task done` against its own slug), the naive
// dirname returns the worktree path, not the project root, which in
// turn makes Done's worktree-remove target a nonexistent
// "<worktree>/.worktrees/<slug>" and the subsequent `git branch -D`
// refuses because the branch is still checked out by the actual
// worktree. `git rev-parse --git-common-dir` returns the main repo's
// .git path regardless of cwd; its dirname is the main repo root.
// Falls back to the naive dirname when tasksDir isn't inside a git
// repo (tests, fresh scaffolds). Mirrors the fix already applied to
// ProjectName in state.go.
func projectRootFromTasksDir(tasksDir string) (string, error) {
	abs, err := filepath.Abs(tasksDir)
	if err != nil {
		return "", err
	}
	naive := filepath.Dir(abs)
	out, err := gitCmd(naive, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return naive, nil
	}
	common := strings.TrimSpace(string(out))
	if common == "" {
		return naive, nil
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(naive, common)
	}
	return filepath.Dir(common), nil
}

func copyBriefToWorktree(tasksDir, worktree, slug string) error {
	src := filepath.Join(tasksDir, slug+".md")
	body, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	rel := filepath.Base(filepath.Clean(tasksDir))
	dst := filepath.Join(worktree, rel, slug+".md")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, body, 0o644)
}

func branchExists(projectRoot, branch string) bool {
	return gitCmd(projectRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// slugServicesDir returns the per-slug services state dir:
// `<XDG_STATE_HOME>/spore/services/<slug>`, falling back to
// `$HOME/.local/state/spore/services/<slug>` when XDG_STATE_HOME is
// unset. Done uses this to clean up the worker's PG / Redis / etc.
// state directory written by the consumer's bin/with-services.
func slugServicesDir(slug string) (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("HOME and XDG_STATE_HOME both unset")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "spore", "services", slug), nil
}
