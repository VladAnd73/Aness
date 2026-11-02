package tokenmonitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coordinatortm "github.com/versality/spore/internal/coordinator/tokenmonitor"
)

func TestIsCoordinator(t *testing.T) {
	cases := []struct {
		inbox    string
		stateDir string
		want     bool
	}{
		{"/state/coord", "/state/coord", true},
		{"/state/coord/some/inbox", "/state/coord", true},
		{"/state/workers/slug/inbox", "/state/coord", false},
		{"", "/state/coord", false},
	}
	for _, tc := range cases {
		cfg := Config{Inbox: tc.inbox, CoordinatorStateDir: tc.stateDir}
		if got := cfg.IsCoordinator(); got != tc.want {
			t.Errorf("IsCoordinator(%q, %q) = %v, want %v", tc.inbox, tc.stateDir, got, tc.want)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := []struct {
		inbox string
		want  string
	}{
		{"/state/workers/my-slug/inbox", "my-slug"},
		{"/anything/abc/inbox", "abc"},
		{"", ""},
		{"/", ""},
	}
	for _, tc := range cases {
		cfg := Config{Inbox: tc.inbox}
		if got := cfg.Slug(); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.inbox, got, tc.want)
		}
	}
}

func TestDefaultWrapCapIsTheCoordinatorHardCap(t *testing.T) {
	if got := (Config{}).Defaults().WrapCap; got != coordinatortm.DefaultHardCap {
		t.Errorf("WrapCap = %d, want %d", got, coordinatortm.DefaultHardCap)
	}
}

func TestCheckSkipsCoordinator(t *testing.T) {
	stateDir := t.TempDir()
	cfg := Config{
		Inbox:               filepath.Join(stateDir, "myproject", "inbox"),
		CoordinatorStateDir: stateDir,
	}
	if got := Check(cfg, HookPayload{}); got.Level != "skip" {
		t.Errorf("expected skip for coordinator inbox, got %s", got.Level)
	}
}

func TestCheckSkipsEmptyInbox(t *testing.T) {
	if got := Check(Config{}, HookPayload{}); got.Level != "skip" {
		t.Errorf("expected skip for empty inbox, got %s", got.Level)
	}
}

func TestCheckSkipsBadInboxLayout(t *testing.T) {
	cfg := Config{
		Inbox:               "/inbox",
		CoordinatorStateDir: "/never",
	}
	if got := Check(cfg, HookPayload{}); got.Level != "skip" {
		t.Errorf("expected skip for bad inbox layout, got %s", got.Level)
	}
}

func TestCheckOk(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":50000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Inbox:               filepath.Join(dir, "workers", "test-slug", "inbox"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
	}
	got := Check(cfg, HookPayload{SessionID: "s", TranscriptPath: transcriptFile})
	if got.Level != "ok" {
		t.Errorf("Level = %s, want ok", got.Level)
	}
	if got.ShouldFire {
		t.Error("ShouldFire = true, want false")
	}
	if got.Slug != "test-slug" {
		t.Errorf("Slug = %q, want %q", got.Slug, "test-slug")
	}
}

func TestCheckWrapAtDefault(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":400000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Inbox:               filepath.Join(dir, "workers", "wrap-slug", "inbox"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
	}
	got := Check(cfg, HookPayload{TranscriptPath: transcriptFile})
	if got.Level != "wrap" || !got.ShouldFire {
		t.Errorf("Level = %s ShouldFire = %v, want wrap/true", got.Level, got.ShouldFire)
	}
	if got.WrapCap != coordinatortm.DefaultHardCap {
		t.Errorf("WrapCap = %d, want %d", got.WrapCap, coordinatortm.DefaultHardCap)
	}
	if !strings.Contains(got.Message, "wrap-slug") {
		t.Errorf("Message missing slug: %q", got.Message)
	}
}

func TestCheckWrapOverride(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":50000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Inbox:               filepath.Join(dir, "workers", "tiny", "inbox"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
		WrapCap:             40000,
	}
	got := Check(cfg, HookPayload{TranscriptPath: transcriptFile})
	if got.Level != "wrap" {
		t.Errorf("Level = %s, want wrap", got.Level)
	}
	if got.WrapCap != 40000 {
		t.Errorf("WrapCap = %d, want 40000", got.WrapCap)
	}
}

func TestCheckMissingTranscript(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Inbox:               filepath.Join(dir, "workers", "slug", "inbox"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
	}
	t.Setenv("HOME", dir)
	t.Setenv("CLAUDE_PROJECT_DIR", "/no/such")
	got := Check(cfg, HookPayload{})
	if got.Level != "skip" {
		t.Errorf("Level = %s, want skip", got.Level)
	}
}
