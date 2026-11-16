// Package tokenmonitor is the worker-side claude-code Stop-hook helper
// that watches a worker's context budget. It reads the hook payload on
// stdin (session_id + transcript_path), parses the latest assistant
// message's usage block from the transcript, and on threshold crossing
// fires a wrap-up reminder so the worker can flush progress to its
// tasks/<slug>.md and let the fleet reconciler resume it.
//
// The threshold is the coordinator hard cap (see
// internal/coordinator/tokenmonitor), so workers and coordinators stop
// at the same context size. The CLI resolves it; zero here falls back
// to the coordinator default.
//
// The worker monitor skips any session whose inbox is under the
// coordinator state dir (those are owned by the coordinator monitor)
// and any session with no inbox set, unless the session is a role
// pane.
//
// Role panes (spawned by fleet rolespawn with SPORE_ROLE and no inbox)
// are metered at the same cap but get role wrap text: commit to the
// task branch, write the step's output if it is complete, end the
// role session. The loop driver, not the reconciler, respawns them.
package tokenmonitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coordinatortm "github.com/versality/spore/internal/coordinator/tokenmonitor"
	"github.com/versality/spore/internal/transcript"
)

type Config struct {
	WrapCap             int
	Inbox               string
	CoordinatorStateDir string

	// Role-pane fields, from the rolespawn session env. A non-empty
	// Role routes Check to the role path regardless of Inbox, which a
	// role pane may inherit from the spawning environment.
	Role        string
	RoleSlug    string
	RoleSession string
	TaskDir     string
}

type CheckResult struct {
	Ctx        int    `json:"ctx"`
	WrapCap    int    `json:"wrap_cap"`
	Slug       string `json:"slug,omitempty"`
	Level      string `json:"level"`
	Message    string `json:"message,omitempty"`
	ShouldFire bool   `json:"should_fire"`
}

type HookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

func (c Config) Defaults() Config {
	if c.WrapCap <= 0 {
		c.WrapCap = coordinatortm.DefaultHardCap
	}
	if c.CoordinatorStateDir == "" {
		c.CoordinatorStateDir = defaultCoordinatorStateDir()
	}
	return c
}

func defaultCoordinatorStateDir() string {
	if d := os.Getenv("SPORE_COORDINATOR_STATE_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "spore", "coordinator")
}

// IsCoordinator returns true if the inbox is under the coordinator
// state dir; the coordinator token monitor handles those.
func (c Config) IsCoordinator() bool {
	if c.Inbox == "" {
		return false
	}
	stateRoot := strings.TrimRight(c.CoordinatorStateDir, "/")
	if stateRoot == "" {
		return false
	}
	return c.Inbox == stateRoot || strings.HasPrefix(c.Inbox, stateRoot+"/")
}

// Slug returns the worker slug parsed from the inbox layout
// <state>/<slug>/inbox. Returns "" when the layout doesn't match.
func (c Config) Slug() string {
	if c.Inbox == "" {
		return ""
	}
	parent := filepath.Base(filepath.Dir(c.Inbox))
	if parent == "." || parent == "/" || parent == "" {
		return ""
	}
	return parent
}

// Check reads the transcript, sums context tokens, and decides whether
// the worker has crossed its wrap cap. When over the cap, ShouldFire is
// true and Message is the wrap-up reminder for the worker to flush to
// tasks/<slug>.md and self-kill so the reconciler can respawn it.
func Check(cfg Config, payload HookPayload) CheckResult {
	cfg = cfg.Defaults()

	if cfg.IsRolePane() {
		return checkRolePane(cfg, payload)
	}

	if cfg.Inbox == "" || cfg.IsCoordinator() {
		return CheckResult{Level: "skip"}
	}

	slug := cfg.Slug()
	if slug == "" {
		return CheckResult{Level: "skip"}
	}

	tpath := payload.TranscriptPath
	if tpath == "" || !fileExists(tpath) {
		tpath = transcript.FindFallbackTranscript()
	}
	if tpath == "" {
		return CheckResult{Level: "skip", Slug: slug}
	}

	wrap := cfg.WrapCap
	ctx := transcript.SumContextTokens(tpath)
	result := CheckResult{
		Ctx:     ctx,
		WrapCap: wrap,
		Slug:    slug,
	}
	if ctx <= 0 {
		result.Level = "ok"
		return result
	}
	if ctx < wrap {
		result.Level = "ok"
		return result
	}

	result.Level = "wrap"
	result.ShouldFire = true
	result.Message = fmt.Sprintf(
		"WORKER TOKEN MONITOR (wrap): context %d tokens >= wrap cap %d.\n"+
			"Wrap up NOW:\n"+
			"  1. Flush in-flight progress to tasks/%s.md so the next worker boots from it.\n"+
			"  2. If you have a blocker for the coordinator, send it before exiting.\n"+
			"  3. Run: tmux kill-session -t \"$(tmux display-message -p '#S')\"\n"+
			"The fleet reconciler resumes a fresh worker with the same worktree on the next pass.",
		ctx, wrap, slug)
	return result
}

// IsRolePane reports whether the session is a loop role pane. The
// agent.env loader reserves SPORE_*, so only rolespawn sets Role.
func (c Config) IsRolePane() bool {
	return c.Role != ""
}

// roleRespawnLine states the respawn contract: the loop driver finds
// the step's session missing on its next tick and spawns a fresh
// agent at the same step, without a respawn marker or cap use.
const roleRespawnLine = "The loop driver resumes a fresh agent at the same step on its next tick;\n" +
	"it resumes from the task branch and the artifacts on disk."

// checkRolePane is the role-pane variant of Check. It never falls back
// to the newest transcript under the cwd: every pane of a task shares
// the task worktree, so that guess could meter a sibling pane.
func checkRolePane(cfg Config, payload HookPayload) CheckResult {
	slug := cfg.RoleSlug
	if slug == "" && cfg.TaskDir != "" {
		slug = filepath.Base(cfg.TaskDir)
	}
	if slug == "" || payload.TranscriptPath == "" || !fileExists(payload.TranscriptPath) {
		return CheckResult{Level: "skip", Slug: slug}
	}

	wrap := cfg.WrapCap
	ctx := transcript.SumContextTokens(payload.TranscriptPath)
	result := CheckResult{Ctx: ctx, WrapCap: wrap, Slug: slug, Level: "ok"}
	if ctx <= 0 || ctx < wrap {
		return result
	}

	kill := `tmux kill-session -t "$(tmux display-message -p '#S')"`
	if cfg.RoleSession != "" {
		kill = fmt.Sprintf("tmux kill-session -t %q", cfg.RoleSession)
	}
	result.Level = "wrap"
	result.ShouldFire = true
	result.Message = fmt.Sprintf(
		"ROLE TOKEN MONITOR (wrap): context %d tokens >= wrap cap %d on role %s.\n"+
			"Wrap up NOW:\n"+
			"  1. Commit in-flight work to the wt/%s branch now; a WIP commit is fine.\n"+
			"  2. If your step's output is complete, write it under $SPORE_TASK_DIR;\n"+
			"     otherwise leave it unwritten so the respawned agent redoes the step.\n"+
			"  3. If you have a blocker for the coordinator, send it before exiting.\n"+
			"  4. Run: %s\n"+
			"%s",
		ctx, wrap, cfg.Role, slug, kill, roleRespawnLine)
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
