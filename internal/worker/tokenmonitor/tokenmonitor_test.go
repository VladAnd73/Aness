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

func writeTranscript(t *testing.T, tokens string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":` + tokens + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func roleConfig(dir string) Config {
	return Config{
		WrapCap:             1000,
		Role:                "engineer",
		RoleSlug:            "feature-x",
		RoleSession:         "spore-role/demo/feature-x/engineer",
		TaskDir:             filepath.Join(dir, "roles", "feature-x"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
	}
}

func TestCheckRolePaneWrap(t *testing.T) {
	dir := t.TempDir()
	cfg := roleConfig(dir)
	got := Check(cfg, HookPayload{TranscriptPath: writeTranscript(t, "500000")})
	if got.Level != "wrap" || !got.ShouldFire {
		t.Fatalf("Level = %s ShouldFire = %v, want wrap/true", got.Level, got.ShouldFire)
	}
	for _, want := range []string{
		"ROLE TOKEN MONITOR",
		"wt/feature-x",
		"$SPORE_TASK_DIR",
		`tmux kill-session -t "spore-role/demo/feature-x/engineer"`,
		"loop driver",
	} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("Message missing %q:\n%s", want, got.Message)
		}
	}
	for _, bad := range []string{"tasks/feature-x.md", "fleet reconciler"} {
		if strings.Contains(got.Message, bad) {
			t.Errorf("Message carries worker wording %q:\n%s", bad, got.Message)
		}
	}
}

func TestCheckRolePaneIgnoresLeakedInbox(t *testing.T) {
	dir := t.TempDir()
	cfg := roleConfig(dir)
	cfg.Inbox = filepath.Join(dir, "workers", "leaked", "inbox")
	got := Check(cfg, HookPayload{TranscriptPath: writeTranscript(t, "500000")})
	if !got.ShouldFire || !strings.Contains(got.Message, "ROLE TOKEN MONITOR") {
		t.Fatalf("want role wrap text despite a leaked inbox, got %+v", got)
	}
	if strings.Contains(got.Message, "leaked") {
		t.Errorf("Message names the leaked inbox slug:\n%s", got.Message)
	}
}

func TestCheckRolePaneUnderCap(t *testing.T) {
	dir := t.TempDir()
	got := Check(roleConfig(dir), HookPayload{TranscriptPath: writeTranscript(t, "500")})
	if got.ShouldFire || got.Level != "ok" {
		t.Fatalf("Level = %s ShouldFire = %v, want ok/false", got.Level, got.ShouldFire)
	}
}

func TestCheckRolePaneWithoutSessionNamesOwnSession(t *testing.T) {
	dir := t.TempDir()
	cfg := roleConfig(dir)
	cfg.RoleSession = ""
	got := Check(cfg, HookPayload{TranscriptPath: writeTranscript(t, "500000")})
	if !strings.Contains(got.Message, `tmux kill-session -t "$(tmux display-message -p '#S')"`) {
		t.Errorf("Message lacks the own-session kill fallback:\n%s", got.Message)
	}
}

func TestCheckWorkerWrapMessageUnchanged(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		WrapCap:             1000,
		Inbox:               filepath.Join(dir, "workers", "wrap-slug", "inbox"),
		CoordinatorStateDir: filepath.Join(dir, "coord"),
	}
	got := Check(cfg, HookPayload{TranscriptPath: writeTranscript(t, "500000")})
	want := "WORKER TOKEN MONITOR (wrap): context 500000 tokens >= wrap cap 1000.\n" +
		"Wrap up NOW:\n" +
		"  1. Flush in-flight progress to tasks/wrap-slug.md so the next worker boots from it.\n" +
		"  2. If you have a blocker for the coordinator, send it before exiting.\n" +
		"  3. Run: tmux kill-session -t \"$(tmux display-message -p '#S')\"\n" +
		"The fleet reconciler resumes a fresh worker with the same worktree on the next pass."
	if got.Message != want || got.Level != "wrap" {
		t.Errorf("worker wrap changed:\ngot  %q\nwant %q", got.Message, want)
	}
}
