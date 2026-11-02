package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type tokenMonitorRun struct {
	claudeProjectDir string
	sporeProjectRoot string
	worker           bool
}

func (r tokenMonitorRun) at(t *testing.T, ctx int) (int, string) {
	t.Helper()
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":` +
		strconv.Itoa(ctx) + `,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "coord")
	inbox := filepath.Join(stateDir, "inbox")
	if r.worker {
		inbox = filepath.Join(dir, "workers", "w", "inbox")
	}
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", stateDir)
	t.Setenv("SPORE_TASK_INBOX", inbox)
	t.Setenv("CLAUDE_PROJECT_DIR", r.claudeProjectDir)
	t.Setenv("SPORE_PROJECT_ROOT", r.sporeProjectRoot)

	origIn, origErr := os.Stdin, os.Stderr
	t.Cleanup(func() { os.Stdin, os.Stderr = origIn, origErr })
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inW.Write([]byte(`{"session_id":"sid","transcript_path":"` + transcriptFile + `"}`)); err != nil {
		t.Fatal(err)
	}
	inW.Close()
	os.Stdin = inR
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = errW

	code := runCoordinatorTokenMonitor(nil)

	errW.Close()
	var buf bytes.Buffer
	io.Copy(&buf, errR)
	return code, buf.String()
}

func writeSporeToml(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func clearCapEnv(t *testing.T) {
	t.Setenv("SPORE_COORDINATOR_SOFT_CAP", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
}

func TestCoordinatorCapsDefaultLeaveRoomPast300k(t *testing.T) {
	clearCapEnv(t)
	run := tokenMonitorRun{claudeProjectDir: t.TempDir()}
	code, stderr := run.at(t, 300000)
	if code != 0 {
		t.Fatalf("300k with default caps: exit %d, want 0 (stderr=%q)", code, stderr)
	}
	code, stderr = run.at(t, 400000)
	if code != 2 || !strings.Contains(stderr, "400000") {
		t.Fatalf("400k with default caps: exit %d, want 2 naming the 400000 hard cap (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsComeFromSporeToml(t *testing.T) {
	clearCapEnv(t)
	root := writeSporeToml(t, "[coordinator]\nsoft_cap = 100000\nhard_cap = 120000\n")
	code, stderr := tokenMonitorRun{claudeProjectDir: root}.at(t, 125000)
	if code != 2 || !strings.Contains(stderr, "120000") {
		t.Fatalf("125k against a 120k hard cap from spore.toml: exit %d (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsEnvBeatsSporeToml(t *testing.T) {
	root := writeSporeToml(t, "[coordinator]\nsoft_cap = 500000\nhard_cap = 600000\n")
	t.Setenv("SPORE_COORDINATOR_SOFT_CAP", "100000")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "120000")
	code, stderr := tokenMonitorRun{claudeProjectDir: root}.at(t, 125000)
	if code != 2 || !strings.Contains(stderr, "120000") {
		t.Fatalf("125k with env hard cap 120k over toml 600k: exit %d, want 2 (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsOnlyHardSetStillWarnsSoftFirst(t *testing.T) {
	clearCapEnv(t)
	root := writeSporeToml(t, "[coordinator]\nhard_cap = 250000\n")
	code, stderr := tokenMonitorRun{claudeProjectDir: root}.at(t, 240000)
	if code != 2 || !strings.Contains(stderr, "(soft)") {
		t.Fatalf("240k with only hard_cap = 250000: exit %d, want the soft warning (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsFoundViaSporeProjectRootFromASubdir(t *testing.T) {
	clearCapEnv(t)
	root := writeSporeToml(t, "[coordinator]\nsoft_cap = 100000\nhard_cap = 120000\n")
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	code, stderr := tokenMonitorRun{sporeProjectRoot: root}.at(t, 125000)
	if code != 2 || !strings.Contains(stderr, "120000") {
		t.Fatalf("SPORE_PROJECT_ROOT names the project, cwd is a subdir: exit %d, want 2 at 120000 (stderr=%q)", code, stderr)
	}
}

func TestNonCoordinatorSessionDoesNotReadSporeToml(t *testing.T) {
	clearCapEnv(t)
	root := writeSporeToml(t, "[coordinator]\nsome_future_key = 1\n")
	code, stderr := tokenMonitorRun{claudeProjectDir: root, worker: true}.at(t, 900000)
	if code != 0 || stderr != "" {
		t.Fatalf("worker session: exit %d stderr %q, want silent exit 0", code, stderr)
	}
}
