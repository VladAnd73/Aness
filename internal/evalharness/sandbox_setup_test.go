package evalharness

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/versality/spore/internal/isolate"
)

// fakeBwrap puts a bwrap on PATH that reports like real bwrap 0.12:
// child-pid on the status fd right after clone, before any setup, and
// exit-code only once the command ran. A refusing one then fails setup
// the way an unprivileged host does at runtime (the Available probe,
// which has no --unshare-pid, still passes).
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
	script := "#!/bin/sh\n" + `fd=
for a in "$@"; do
  [ "$prev" = --json-status-fd ] || [ "$prev" = --info-fd ] && fd=$a
  prev=$a
done
[ -n "$fd" ] && eval "echo '{ \"child-pid\": 2 }' >&$fd"
` + refusal + `while [ $# -gt 0 ]; do
  case "$1" in
    --) shift; "$@"; code=$?; [ -n "$fd" ] && eval "echo '{ \"exit-code\": $code }' >&$fd"; exit $code ;;
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
	return exitingAgentIn(t, t.TempDir(), code)
}

// exitingAgentIn writes the agent into dir; a real sandbox sees only
// its work dir, not the host's /tmp.
func exitingAgentIn(t *testing.T, dir, code string) string {
	t.Helper()
	p := filepath.Join(dir, "agent")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho reply\nexit "+code+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func sandboxReq(t *testing.T) SpawnRequest {
	t.Helper()
	root := t.TempDir()
	spec := isolate.Spec{Work: root, Home: filepath.Join(root, "home"), Env: []string{"PATH=/bin"}}
	if err := os.MkdirAll(spec.Home, 0o700); err != nil {
		t.Fatal(err)
	}
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

// Real bwrap, failing a mount after it reported the child's pid.
func TestIsolatedSpawnerRealBwrapSetupFailureIsNoAgentRun(t *testing.T) {
	requireSandbox(t)
	req := sandboxReq(t)
	req.Isolate.Extra = []isolate.Bind{{Src: filepath.Join(t.TempDir(), "missing"), Dst: "/missing"}}
	res, err := IsolatedSpawner{Binary: exitingAgentIn(t, req.WorkDir, "0")}.Spawn(context.Background(), req)
	if err == nil || res.ExitCode != 0 {
		t.Errorf("Spawn with a missing bind source = exit %d, err %v; want exit 0 and an error", res.ExitCode, err)
	}
}

func TestIsolatedSpawnerRealBwrapAgentExitKeepsItsCode(t *testing.T) {
	requireSandbox(t)
	req := sandboxReq(t)
	res, err := IsolatedSpawner{Binary: exitingAgentIn(t, req.WorkDir, "3")}.Spawn(context.Background(), req)
	if res.ExitCode != 3 {
		t.Errorf("agent exit code %d (err %v), want 3", res.ExitCode, err)
	}
}
