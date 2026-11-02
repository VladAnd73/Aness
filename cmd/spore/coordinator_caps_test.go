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

func runCoordinatorTokenMonitorAt(t *testing.T, projectRoot string, ctx int) (int, string) {
	t.Helper()
	dir := t.TempDir()
	transcriptFile := filepath.Join(dir, "session.jsonl")
	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":` +
		strconv.Itoa(ctx) + `,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(transcriptFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "coord")
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", stateDir)
	t.Setenv("SPORE_TASK_INBOX", filepath.Join(stateDir, "inbox"))
	t.Setenv("CLAUDE_PROJECT_DIR", projectRoot)

	origIn, origErr := os.Stdin, os.Stderr
	t.Cleanup(func() { os.Stdin, os.Stderr = origIn, origErr })
	inR, inW, _ := os.Pipe()
	inW.Write([]byte(`{"session_id":"sid","transcript_path":"` + transcriptFile + `"}`))
	inW.Close()
	os.Stdin = inR
	errR, errW, _ := os.Pipe()
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

func TestCoordinatorCapsDefaultLeaveRoomPast300k(t *testing.T) {
	t.Setenv("SPORE_COORDINATOR_SOFT_CAP", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
	code, stderr := runCoordinatorTokenMonitorAt(t, t.TempDir(), 300000)
	if code != 0 {
		t.Fatalf("300k with default caps: exit %d, want 0 (stderr=%q)", code, stderr)
	}
	code, stderr = runCoordinatorTokenMonitorAt(t, t.TempDir(), 400000)
	if code != 2 || !strings.Contains(stderr, "400000") {
		t.Fatalf("400k with default caps: exit %d, want 2 naming the 400000 hard cap (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsComeFromSporeToml(t *testing.T) {
	t.Setenv("SPORE_COORDINATOR_SOFT_CAP", "")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "")
	root := writeSporeToml(t, "[coordinator]\nsoft_cap = 100000\nhard_cap = 120000\n")
	code, stderr := runCoordinatorTokenMonitorAt(t, root, 125000)
	if code != 2 || !strings.Contains(stderr, "120000") {
		t.Fatalf("125k against a 120k hard cap from spore.toml: exit %d (stderr=%q)", code, stderr)
	}
}

func TestCoordinatorCapsEnvBeatsSporeToml(t *testing.T) {
	root := writeSporeToml(t, "[coordinator]\nsoft_cap = 100000\nhard_cap = 120000\n")
	t.Setenv("SPORE_COORDINATOR_SOFT_CAP", "500000")
	t.Setenv("SPORE_COORDINATOR_HARD_CAP", "600000")
	code, stderr := runCoordinatorTokenMonitorAt(t, root, 125000)
	if code != 0 {
		t.Fatalf("125k with env caps 500k/600k: exit %d, want 0 (stderr=%q)", code, stderr)
	}
}
