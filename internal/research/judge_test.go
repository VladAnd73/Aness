package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scriptJudge(t *testing.T, body string) CommandJudge {
	t.Helper()
	p := filepath.Join(t.TempDir(), "judge.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return CommandJudge{Argv: []string{p, "fixed-arg"}}
}

// argsThenOut writes the script's own argv to args.txt next to --out
// and then the result JSON to the --out path.
const argsThenOut = `out=""
prev=""
for a in "$@"; do
  [ "$prev" = "--out" ] && out="$a"
  prev="$a"
done
echo "$@" > "$(dirname "$out")/args.txt"
`

func TestCommandJudgeArgvAndDecode(t *testing.T) {
	j := scriptJudge(t, argsThenOut+`echo '{"score": 66.5, "runs": 6, "passed": 4, "crashes": 1, "cost_usd": 0.25}' > "$out"
echo "stdout is not the result"
`)
	dir := t.TempDir()
	res, err := j.Run(context.Background(), "/cand/SKILL.md", "main", filepath.Join(dir, "r.json"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Score != 66.5 || res.Runs != 6 || res.Crashes != 1 || res.CostUSD != 0.25 {
		t.Errorf("res = %+v", res)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	want := "fixed-arg --candidate /cand/SKILL.md --set main --out " + filepath.Join(dir, "r.json")
	if strings.TrimSpace(string(args)) != want {
		t.Errorf("args = %q, want %q", args, want)
	}
}

func TestCommandJudgeScoreOutOfRangeIsError(t *testing.T) {
	for _, score := range []string{"-1", "100.5"} {
		j := scriptJudge(t, argsThenOut+`echo '{"score": `+score+`}' > "$out"`+"\n")
		if _, err := j.Run(context.Background(), "c", "main", filepath.Join(t.TempDir(), "r.json")); err == nil {
			t.Errorf("score %s: want an error", score)
		}
	}
}

func TestCommandJudgeFailuresAreErrors(t *testing.T) {
	for name, body := range map[string]string{
		"non-zero exit": argsThenOut + `echo '{"score": 50}' > "$out"; exit 1` + "\n",
		"no result":     "exit 0\n",
		"bad json":      argsThenOut + `echo 'nope' > "$out"` + "\n",
	} {
		if _, err := scriptJudge(t, body).Run(context.Background(), "c", "main", filepath.Join(t.TempDir(), "r.json")); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestCommandJudgeCancelKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	j := scriptJudge(t, "sleep 60 &\necho $! > "+pidFile+"\nwait\n")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := j.Run(ctx, "c", "main", filepath.Join(dir, "r.json"))
		done <- err
	}()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Errorf("cancelled Run returned nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("Run did not return after cancel")
	}
	b, _ := os.ReadFile(pidFile)
	pid := strings.TrimSpace(string(b))
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat("/proc/" + pid); err == nil {
		stat, _ := os.ReadFile("/proc/" + pid + "/stat")
		if !strings.Contains(string(stat), ") Z ") {
			t.Errorf("judge grandchild %s still alive after cancel", pid)
		}
	}
}
