package fleet

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// liveTmuxTmpdir snapshots TMUX_TMPDIR at package init, before
// TestMain redirects every tmux call onto an isolated per-process
// socket. The live smoke below restores it so sendWakeVerified
// reaches the operator's real tmux server, where the target session
// lives.
var liveTmuxTmpdir = os.Getenv("TMUX_TMPDIR")

// TestLiveWakeDelivery drives sendWakeVerified against a real tmux
// session running a live claude-code TUI. Gated on
// SPORE_LIVE_WAKE_SESSION so the ordinary test run stays hermetic:
//
//	tmux new-session -d -s wake-smoke 'claude --dangerously-skip-permissions'
//	# wait ~10s for the TUI to render, accept the trust dialog if any
//	SPORE_LIVE_WAKE_SESSION=wake-smoke go test ./internal/fleet/ -run TestLiveWakeDelivery -v
func TestLiveWakeDelivery(t *testing.T) {
	session := os.Getenv("SPORE_LIVE_WAKE_SESSION")
	if session == "" {
		t.Skip("SPORE_LIVE_WAKE_SESSION not set; skipping live tmux wake smoke")
	}
	t.Setenv("TMUX_TMPDIR", liveTmuxTmpdir)
	if !hasSession(session) {
		t.Fatalf("tmux session %q not found", session)
	}
	pane, err := capturePane(session)
	if err != nil {
		t.Fatalf("capture-pane before send: %v", err)
	}
	if !hasInputPrompt(pane) {
		t.Fatalf("pre-send gate rejected the pane (no input prompt); capture:\n%s", pane)
	}

	msg := fmt.Sprintf("Live wake smoke %d: reply with the single word ack.", time.Now().UnixNano())
	delivered, err := sendWakeVerified(session, msg)
	if err != nil {
		t.Fatalf("sendWakeVerified: %v", err)
	}
	if !delivered {
		t.Fatalf("wake not delivered: sendWakeVerified returned false")
	}

	pane, err = capturePane(session)
	if err != nil {
		t.Fatalf("capture-pane after send: %v", err)
	}
	frag := alnumOnly(msg)
	if len(frag) > wakePendingFragLen {
		frag = frag[:wakePendingFragLen]
	}
	if !strings.Contains(alnumOnly(pane), frag) {
		t.Errorf("message not echoed into the transcript; capture:\n%s", pane)
	}
	if !wakeSubmitted(pane, msg) {
		t.Errorf("message still pending in the input region; capture:\n%s", pane)
	}
	t.Logf("delivered=%v; post-send capture tail:\n%s", delivered, tailLines(pane, 12))
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
