package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureWorkerTokenMonitor runs runWorkerTokenMonitor with stdin
// piped from input and stdout/stderr captured. It mirrors the helper
// pattern used by other cmd-level tests so the test never touches the
// real os.Stdout / os.Stderr.
func captureWorkerTokenMonitor(t *testing.T, input string) (code int, stderr string) {
	t.Helper()

	origIn, origOut, origErr := os.Stdin, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Stdin, os.Stdout, os.Stderr = origIn, origOut, origErr })

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	if _, err := inW.Write([]byte(input)); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	inW.Close()
	os.Stdin = inR

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW

	code = runWorkerTokenMonitor(nil)

	outW.Close()
	errW.Close()
	io.Copy(io.Discard, outR)
	var errBuf bytes.Buffer
	io.Copy(&errBuf, errR)
	return code, errBuf.String()
}

func TestRunWorkerTokenMonitorSkipNoInbox(t *testing.T) {
	t.Setenv("SPORE_TASK_INBOX", "")
	code, _ := captureWorkerTokenMonitor(t, `{"session_id":"s","transcript_path":""}`)
	if code != 0 {
		t.Fatalf("expected exit 0 with no inbox, got %d", code)
	}
}

func TestRunWorkerTokenMonitorWrap(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":400000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SPORE_TASK_INBOX", filepath.Join(dir, "workers", "wrapme", "inbox"))
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", filepath.Join(dir, "coord"))
	t.Setenv("SPORE_PROJECT_ROOT", dir)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
	t.Setenv("SPORE_WORKER_TOKEN_WRAP", "")

	payload := `{"session_id":"sid","transcript_path":"` + transcriptFile + `"}`
	code, stderr := captureWorkerTokenMonitor(t, payload)
	if code != 2 {
		t.Fatalf("expected exit 2 on wrap, got %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, "WORKER TOKEN MONITOR") {
		t.Errorf("expected WORKER TOKEN MONITOR in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "wrapme") {
		t.Errorf("expected slug in stderr, got %q", stderr)
	}
}

func TestRunWorkerTokenMonitorOk(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":50000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SPORE_TASK_INBOX", filepath.Join(dir, "workers", "ok", "inbox"))
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", filepath.Join(dir, "coord"))
	t.Setenv("SPORE_PROJECT_ROOT", dir)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
	t.Setenv("SPORE_WORKER_TOKEN_WRAP", "")

	payload := `{"session_id":"sid","transcript_path":"` + transcriptFile + `"}`
	code, stderr := captureWorkerTokenMonitor(t, payload)
	if code != 0 {
		t.Fatalf("expected exit 0 below cap, got %d (stderr=%q)", code, stderr)
	}
}

func TestRunWorkerDispatch(t *testing.T) {
	if code := runWorker(nil); code != 2 {
		t.Errorf("runWorker(nil) = %d, want 2", code)
	}
	if code := runWorker([]string{"bogus"}); code != 2 {
		t.Errorf("runWorker(bogus) = %d, want 2", code)
	}
}

func setRolePaneEnv(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("SPORE_TASK_INBOX", "")
	t.Setenv("SPORE_ROLE", "engineer")
	t.Setenv("SPORE_TASK_SLUG", "feature-x")
	t.Setenv("SPORE_TASK_DIR", filepath.Join(dir, "roles", "feature-x"))
	t.Setenv("SPORE_ROLE_SESSION", "spore-role/demo/feature-x/engineer")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", filepath.Join(dir, "coord"))
	t.Setenv("SPORE_PROJECT_ROOT", dir)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
	t.Setenv("SPORE_WORKER_TOKEN_WRAP", "1000")
}

func writeTokenTranscript(t *testing.T, dir, tokens string) string {
	t.Helper()
	p := filepath.Join(dir, "session.jsonl")
	line := `{"role":"assistant","usage":{"input_tokens":` + tokens + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunWorkerTokenMonitorRolePaneWrap(t *testing.T) {
	dir := t.TempDir()
	setRolePaneEnv(t, dir)
	payload := `{"session_id":"sid","transcript_path":"` + writeTokenTranscript(t, dir, "500000") + `"}`
	code, stderr := captureWorkerTokenMonitor(t, payload)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, `tmux kill-session -t "spore-role/demo/feature-x/engineer"`) {
		t.Errorf("stderr does not name the role session:\n%s", stderr)
	}
	for _, bad := range []string{"tasks/feature-x.md", "fleet reconciler"} {
		if strings.Contains(stderr, bad) {
			t.Errorf("stderr carries worker wording %q:\n%s", bad, stderr)
		}
	}
}

func TestRunWorkerTokenMonitorRolePaneUnderCap(t *testing.T) {
	dir := t.TempDir()
	setRolePaneEnv(t, dir)
	payload := `{"session_id":"sid","transcript_path":"` + writeTokenTranscript(t, dir, "500") + `"}`
	code, stderr := captureWorkerTokenMonitor(t, payload)
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d stderr = %q, want 0 and silent", code, stderr)
	}
}

func TestRunWorkerTokenMonitorRolePaneDefaultCap(t *testing.T) {
	dir := t.TempDir()
	setRolePaneEnv(t, dir)
	t.Setenv("SPORE_WORKER_TOKEN_WRAP", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "2000")
	payload := `{"session_id":"sid","transcript_path":"` + writeTokenTranscript(t, dir, "3000") + `"}`
	code, stderr := captureWorkerTokenMonitor(t, payload)
	if code != 2 || !strings.Contains(stderr, "wrap cap 2000") {
		t.Fatalf("exit = %d stderr = %q, want 2 at the coordinator hard cap", code, stderr)
	}
}

func TestWorkerTokenMonitorRolePaneE2E(t *testing.T) {
	bin := sharedSporeBin(t)
	dir := t.TempDir()
	env := append(e2eEnv(t),
		"SPORE_ROLE=engineer",
		"SPORE_TASK_SLUG=feature-x",
		"SPORE_TASK_DIR="+filepath.Join(dir, "roles", "feature-x"),
		"SPORE_ROLE_SESSION=spore-role/demo/feature-x/engineer",
		"SPORE_PROJECT_ROOT="+dir,
		"SPORE_WORKER_TOKEN_WRAP=1000",
	)
	payload := `{"session_id":"sid","transcript_path":"` + writeTokenTranscript(t, dir, "500000") + `"}`
	res := runCLI(t, bin, dir, env, payload, "worker", "token-monitor")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr=%q)", res.code, res.stderr)
	}
	for _, want := range []string{"ROLE TOKEN MONITOR", `tmux kill-session -t "spore-role/demo/feature-x/engineer"`} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, res.stderr)
		}
	}
	for _, bad := range []string{"tasks/feature-x.md", "fleet reconciler"} {
		if strings.Contains(res.stderr, bad) {
			t.Errorf("stderr carries worker wording %q:\n%s", bad, res.stderr)
		}
	}
}
