package isolate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireBwrap(t *testing.T) {
	t.Helper()
	if err := Available(); err != nil {
		t.Skipf("bwrap cannot run here (as in the nix build sandbox): %v", err)
	}
}

func liveSpec(t *testing.T) Spec {
	t.Helper()
	root := t.TempDir()
	s := Spec{Work: filepath.Join(root, "work"), Home: filepath.Join(root, "home")}
	for _, d := range []string{s.Work, s.Home} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	s.Env = BaseEnv(s.Home)
	return s
}

func run(t *testing.T, s Spec, argv ...string) (string, error) {
	t.Helper()
	cmd, err := Command(context.Background(), s, argv)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestBwrapHidesOutsidePaths(t *testing.T) {
	requireBwrap(t)
	s := liveSpec(t)
	sibling := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(sibling, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, s, "cat", sibling); err == nil || strings.Contains(out, "outside") {
		t.Errorf("cat %s inside the sandbox = %q, %v; want it unreadable", sibling, out, err)
	}
	home, _ := os.UserHomeDir()
	if out, err := run(t, s, "ls", home); err == nil {
		t.Errorf("ls host home %s inside the sandbox succeeded: %q", home, out)
	}
	in := filepath.Join(s.Work, "in.txt")
	if out, err := run(t, s, "sh", "-c", "echo hi > "+in+" && cat "+in); err != nil || strings.TrimSpace(out) != "hi" {
		t.Errorf("work dir write+read = %q, %v", out, err)
	}
	if b, _ := os.ReadFile(in); string(b) != "hi\n" {
		t.Errorf("work dir write did not reach the host: %q", b)
	}
}

func TestBwrapBlocksOutsideHosts(t *testing.T) {
	requireBwrap(t)
	out, err := run(t, liveSpec(t), "getent", "ahosts", "api.notion.com")
	if err != nil || strings.Fields(out + " x")[0] != "0.0.0.0" {
		t.Errorf("getent ahosts api.notion.com = %q, %v; want 0.0.0.0 first", out, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] != "0.0.0.0" && f[0] != "::" {
			t.Errorf("api.notion.com resolves to a real address %s", f[0])
		}
	}
}

func TestBwrapMountsCredentialsCopyNotHostFile(t *testing.T) {
	requireBwrap(t)
	host := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(host, []byte(`{"claudeAiOauth":{"accessToken":"at-1","refreshToken":"rt-1","expiresAt":9}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	checked := ""
	old := CheckCredentials
	CheckCredentials = func(path string) error { checked = path; return nil }
	t.Cleanup(func() { CheckCredentials = old })

	s := liveSpec(t)
	s.Credentials = host
	dst := filepath.Join(s.Home, ".claude", ".credentials.json")
	out, err := run(t, s, "sh", "-c", "cat "+dst+"; echo; cat "+host+" || echo HOST-HIDDEN; echo x > "+dst+" || echo RO")
	if err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	if checked != host {
		t.Errorf("host credentials not checked before the spawn (checked %q)", checked)
	}
	if !strings.Contains(out, "at-1") || strings.Contains(out, "rt-1") {
		t.Errorf("copy inside = %q; want the access token and no refresh token", out)
	}
	if !strings.Contains(out, "HOST-HIDDEN") || !strings.Contains(out, "RO") {
		t.Errorf("host file readable or copy writable: %q", out)
	}
}

func TestBwrapCancelKillsTheWholeTree(t *testing.T) {
	requireBwrap(t)
	s := liveSpec(t)
	tick := filepath.Join(s.Work, "tick")
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := Command(ctx, s, []string{"sh", "-c", "(while true; do echo t >> " + tick + "; sleep 0.05; done) & sleep 300"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, _ := os.ReadFile(tick); len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background child never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return after cancel")
	}
	time.Sleep(300 * time.Millisecond)
	before, _ := os.ReadFile(tick)
	time.Sleep(500 * time.Millisecond)
	after, _ := os.ReadFile(tick)
	if len(after) != len(before) {
		t.Errorf("a background child outlived the cancelled sandbox (tick grew %d -> %d)", len(before), len(after))
	}
}
