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
// and any session with no inbox set.
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
