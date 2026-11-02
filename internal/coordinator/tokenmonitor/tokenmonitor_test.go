package tokenmonitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsCoordinator(t *testing.T) {
	cases := []struct {
		inbox    string
		stateDir string
		want     bool
	}{
		{"/state/coord", "/state/coord", true},
		{"/state/coord/myproj/inbox", "/state/coord", true},
		{"/state/wt/slug/inbox", "/state/coord", false},
		{"", "/state/coord", false},
	}
	for _, tc := range cases {
		cfg := Config{Inbox: tc.inbox, StateDir: tc.stateDir}
		if got := cfg.IsCoordinator(); got != tc.want {
			t.Errorf("IsCoordinator(%q, %q) = %v, want %v", tc.inbox, tc.stateDir, got, tc.want)
		}
	}
}

func TestCheckSkipsNonCoordinator(t *testing.T) {
	cfg := Config{
		Inbox:    "/some/other/path",
		StateDir: "/state/coord",
	}
	result := Check(cfg, HookPayload{})
	if result.Level != "skip" {
		t.Errorf("expected skip, got %s", result.Level)
	}
}

func TestCheckHardCap(t *testing.T) {
	dir := t.TempDir()
	transcriptDir := filepath.Join(dir, "transcript")
	os.MkdirAll(transcriptDir, 0o700)
	f := filepath.Join(transcriptDir, "session.jsonl")

	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":195000,"output_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	os.WriteFile(f, []byte(line+"\n"), 0o644)

	stateDir := filepath.Join(dir, "state")
	cfg := Config{
		SoftCap:  150000,
		HardCap:  190000,
		StateDir: stateDir,
		Inbox:    stateDir,
	}

	result := Check(cfg, HookPayload{SessionID: "test", TranscriptPath: f})
	if result.Level != "hard" {
		t.Errorf("expected hard, got %s", result.Level)
	}
	if !result.ShouldFire {
		t.Error("expected ShouldFire = true")
	}
	if result.Ctx != 195000 {
		t.Errorf("Ctx = %d, want 195000", result.Ctx)
	}
	assertRespawnPaneMessage(t, result.Message)
}

func TestCheckSoftCap(t *testing.T) {
	dir := t.TempDir()
	transcriptDir := filepath.Join(dir, "transcript")
	os.MkdirAll(transcriptDir, 0o700)
	f := filepath.Join(transcriptDir, "session.jsonl")

	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":160000,"output_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	os.WriteFile(f, []byte(line+"\n"), 0o644)

	stateDir := filepath.Join(dir, "state")
	cfg := Config{
		SoftCap:  150000,
		HardCap:  190000,
		StateDir: stateDir,
		Inbox:    stateDir,
	}

	result := Check(cfg, HookPayload{SessionID: "test-soft", TranscriptPath: f})
	if result.Level != "soft" {
		t.Errorf("expected soft, got %s", result.Level)
	}
	if !result.ShouldFire {
		t.Error("expected ShouldFire = true on first soft crossing")
	}
	assertRespawnPaneMessage(t, result.Message)

	result2 := Check(cfg, HookPayload{SessionID: "test-soft", TranscriptPath: f})
	if result2.Level != "ok" {
		t.Errorf("expected ok on second check (soft marker exists), got %s", result2.Level)
	}
	if result2.ShouldFire {
		t.Error("expected ShouldFire = false on second soft check")
	}
}

func TestCheckOk(t *testing.T) {
	dir := t.TempDir()
	transcriptDir := filepath.Join(dir, "transcript")
	os.MkdirAll(transcriptDir, 0o700)
	f := filepath.Join(transcriptDir, "session.jsonl")

	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":50000,"output_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	os.WriteFile(f, []byte(line+"\n"), 0o644)

	stateDir := filepath.Join(dir, "state")
	cfg := Config{
		SoftCap:  150000,
		HardCap:  190000,
		StateDir: stateDir,
		Inbox:    stateDir,
	}

	result := Check(cfg, HookPayload{SessionID: "test-ok", TranscriptPath: f})
	if result.Level != "ok" {
		t.Errorf("expected ok, got %s", result.Level)
	}
}

// assertRespawnPaneMessage checks the wrap-up message instructs the
// coordinator agent to respawn the pane (preserving the tmux session
// and any attached SSH client) rather than killing the session.
func assertRespawnPaneMessage(t *testing.T, msg string) {
	t.Helper()
	if !strings.Contains(msg, "tmux respawn-pane -k") {
		t.Errorf("message must instruct respawn-pane -k, got:\n%s", msg)
	}
	if !strings.Contains(msg, `"$(tmux display-message -p '#S'):0"`) {
		t.Errorf("message must self-target via display-message, got:\n%s", msg)
	}
	if !strings.Contains(msg, "/usr/local/bin/spore-coordinator-launch") {
		t.Errorf("message must exec the coordinator-launch shim, got:\n%s", msg)
	}
	if strings.Contains(msg, "tmux kill-session") {
		t.Errorf("message must not instruct kill-session (drops attached SSH), got:\n%s", msg)
	}
}

func TestAppendLedger(t *testing.T) {
	dir := t.TempDir()
	ledgerFile := filepath.Join(dir, "ledger.jsonl")
	cfg := Config{
		StateDir:   dir,
		LedgerFile: ledgerFile,
	}
	cfg = cfg.Defaults()
	appendLedger(cfg, "sess1", 100000, false, false)

	content, err := os.ReadFile(ledgerFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Error("expected ledger content")
	}
}

func TestDefaultsGiveCoordinatorsRoomBeforeTheCaps(t *testing.T) {
	cfg := Config{StateDir: t.TempDir()}.Defaults()
	if cfg.SoftCap != 350000 {
		t.Errorf("SoftCap = %d, want 350000", cfg.SoftCap)
	}
	if cfg.HardCap != 400000 {
		t.Errorf("HardCap = %d, want 400000", cfg.HardCap)
	}
}

func TestDefaultsKeepSoftBelowHardWhenOnlyOneCapIsSet(t *testing.T) {
	for _, c := range []Config{{HardCap: 250000}, {SoftCap: 450000}} {
		got := Config{SoftCap: c.SoftCap, HardCap: c.HardCap, StateDir: t.TempDir()}.Defaults()
		if got.SoftCap >= got.HardCap {
			t.Errorf("from %d/%d: Defaults gave soft %d >= hard %d", c.SoftCap, c.HardCap, got.SoftCap, got.HardCap)
		}
	}
}
