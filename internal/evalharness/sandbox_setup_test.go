package evalharness

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/versality/spore/internal/isolate"
)

// fakeBwrap puts a bwrap on PATH. A refusing one fails a real sandbox
// the way an unprivileged host does at runtime (the Available probe
// still passes); a working one reports the sandbox started on
// --info-fd, then runs the command after "--".
func fakeBwrap(t *testing.T, refuse bool) {
	t.Helper()
	dir := t.TempDir()
	refusal := ""
	if refuse {
		refusal = `for a in "$@"; do
  [ "$a" = --unshare-pid ] && { echo 'bwrap: setting up uid map: Permission denied' >&2; exit 1; }
done
`
	}
	script := "#!/bin/sh\n" + refusal + `while [ $# -gt 0 ]; do
  case "$1" in
    --info-fd) eval "echo '{\"child-pid\": 2}' >&$2"; shift 2 ;;
    --) shift; exec "$@" ;;
    *) shift ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func exitingAgent(t *testing.T, code string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho reply\nexit "+code+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func sandboxReq(t *testing.T) SpawnRequest {
	t.Helper()
	root := t.TempDir()
	spec := isolate.Spec{Work: root, Home: filepath.Join(root, "home"), Env: []string{"PATH=/bin"}}
	return SpawnRequest{Prompt: "x", WorkDir: root, Env: spec.Env, Isolate: &spec}
}

// A sandbox that never started means no agent ran: report no exit code
// and an error, so callers fail the judge instead of scoring a crash.
func TestIsolatedSpawnerSandboxRefusalIsNoAgentRun(t *testing.T) {
	fakeBwrap(t, true)
	res, err := IsolatedSpawner{Binary: exitingAgent(t, "0")}.Spawn(context.Background(), sandboxReq(t))
	if err == nil || res.ExitCode != 0 {
		t.Errorf("Spawn under a refusing bwrap = exit %d, err %v; want exit 0 and an error", res.ExitCode, err)
	}
}

func TestIsolatedSpawnerAgentExitKeepsItsCode(t *testing.T) {
	fakeBwrap(t, false)
	res, _ := IsolatedSpawner{Binary: exitingAgent(t, "3")}.Spawn(context.Background(), sandboxReq(t))
	if res.ExitCode != 3 {
		t.Errorf("agent exit code %d, want 3", res.ExitCode)
	}
}

// The grader under a bwrap that refuses at runtime must fail the judge,
// not count as a grader crash that scores the candidate down.
func TestLLMJudgeGraderSandboxRefusalFailsTheJudge(t *testing.T) {
	fakeBwrap(t, true)
	agent := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		return SpawnResult{Stdout: resultLine("reply", 0)}, nil
	}}
	o := isolatedOpts(t, writeJudgedSet(t), agent)
	o.Grader = IsolatedSpawner{Binary: exitingAgent(t, "0")}
	if rep, err := RunJudge(context.Background(), o); err == nil {
		t.Errorf("RunJudge = %+v, nil error; want the grader's sandbox refusal as an error", rep)
	}
}
