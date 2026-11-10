package evalharness

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/isolate"
	"github.com/versality/spore/internal/research/fakemcp"
)

func requireBwrap(t *testing.T) {
	t.Helper()
	if err := isolate.Available(); err != nil {
		t.Skipf("bwrap cannot run here: %v", err)
	}
}

// longLivedCreds points the credentials lookup at a throwaway login
// that needs no refresh, so no test touches the host's real file.
func longLivedCreds(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".credentials.json")
	body, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "at-test", "refreshToken": "rt-test", "expiresAt": time.Now().Add(10 * time.Hour).UnixMilli(),
	}})
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUDGET_CREDS", p)
	return p
}

func isolatedOpts(t *testing.T, set string, sp AgentSpawner) JudgeOptions {
	t.Helper()
	requireBwrap(t)
	longLivedCreds(t)
	o := judgeOpts(t, set, sp)
	t.Setenv(AllowUnsandboxedEnv, "")
	o.Isolation, o.Repeats = "bwrap", 1
	return o
}

func argAfter(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func mcpServerNames(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mcp config %q: %v", path, err)
	}
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("mcp config %s: %v", b, err)
	}
	var names []string
	for n := range cfg.MCPServers {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

func TestJudgeIsolatedArgsUseStrictMCP(t *testing.T) {
	var names []string
	var req SpawnRequest
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		req = r
		names = mcpServerNames(t, argAfter(r.ExtraArgs, "--mcp-config"))
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	o := isolatedOpts(t, "testdata/notion-set/main", sp)
	if _, err := RunJudge(context.Background(), o); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if !slices.Contains(req.ExtraArgs, "--strict-mcp-config") {
		t.Errorf("argv lacks --strict-mcp-config: %v", req.ExtraArgs)
	}
	if strings.Join(names, ",") != "claude_ai_Notion" {
		t.Errorf("mcp config servers = %v, want exactly the declared [claude_ai_Notion]", names)
	}
	if req.Isolate == nil {
		t.Fatal("isolated judge handed the spawner no sandbox spec")
	}
	if !slices.Equal(req.Env, req.Isolate.Env) || !slices.Contains(req.Env, "HOME="+req.Isolate.Home) {
		t.Errorf("env = %v, want the sandbox's own env with HOME at the agent home", req.Env)
	}
}

func TestJudgeIsolatedEmptyMCPConfigWithoutServices(t *testing.T) {
	var names []string
	var cfg string
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		cfg = argAfter(r.ExtraArgs, "--mcp-config")
		names = mcpServerNames(t, cfg)
		return SpawnResult{Stdout: resultLine("OK", 0)}, nil
	}}
	o := isolatedOpts(t, writeSet(t, "pass a"), sp)
	if _, err := RunJudge(context.Background(), o); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if cfg == "" || len(names) != 0 {
		t.Errorf("mcp config %q names %v, want an empty server map", cfg, names)
	}
}

func under(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func TestJudgeIsolatedSpecDoesNotBindScenarioStore(t *testing.T) {
	var reqs []SpawnRequest
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		reqs = append(reqs, r)
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	o := isolatedOpts(t, "testdata/notion-set/main", sp)
	o.FrozenDir = "testdata/notion-set/skill"
	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	set, _ := filepath.Abs(o.SetDir)
	frozen, _ := filepath.Abs(o.FrozenDir)
	cand, _ := filepath.Abs(filepath.Dir(o.Candidate))
	run := firstRun(t, rep)
	if len(reqs) != 1 || reqs[0].Isolate == nil {
		t.Fatalf("requests = %+v", reqs)
	}
	s := reqs[0].Isolate
	srcs := []string{s.Work, s.Home}
	for _, b := range s.Extra {
		srcs = append(srcs, b.Src)
		if b.RW {
			t.Errorf("extra bind %v is writable", b)
		}
	}
	for _, src := range srcs {
		abs, _ := filepath.Abs(src)
		for _, store := range []string{set, frozen, cand} {
			if under(abs, store) {
				t.Errorf("sandbox binds %s, inside the scenario store or skill source %s", abs, store)
			}
		}
		if !under(abs, run.SandboxRoot) {
			t.Errorf("sandbox binds %s, outside the run's own root %s", abs, run.SandboxRoot)
		}
	}
	if under(run.CallsLog, s.Work) || under(run.CallsLog, s.Home) || under(run.TranscriptPath, s.Work) {
		t.Errorf("calls log %s or transcript %s is writable from inside the sandbox", run.CallsLog, run.TranscriptPath)
	}
}

func TestRunJudgeDefaultsToRealFakeServices(t *testing.T) {
	sp := &scriptSpawner{fn: func(ctx context.Context, r SpawnRequest) (SpawnResult, error) {
		b, _ := os.ReadFile(argAfter(r.ExtraArgs, "--mcp-config"))
		var cfg struct {
			MCPServers map[string]struct{ URL string } `json:"mcpServers"`
		}
		_ = json.Unmarshal(b, &cfg)
		callFake(t, cfg.MCPServers["claude_ai_Notion"].URL, "notion-create-pages", map[string]any{"title": "Nightly"})
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	o := judgeOpts(t, "testdata/notion-set/main", sp)
	o.Repeats = 1
	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	calls, _ := fakemcp.ReadCalls(firstRun(t, rep).CallsLog)
	if rep.Passed != 1 || len(calls) != 1 {
		t.Errorf("passed=%d calls=%+v, want the default fake to serve and log the call", rep.Passed, calls)
	}
}

func TestIsolatedSpawnerRunsTheAgentInsideBwrap(t *testing.T) {
	requireBwrap(t)
	longLivedCreds(t)
	root := t.TempDir()
	work, home := filepath.Join(root, "run"), filepath.Join(root, "agent-home")
	for _, d := range []string{work, home} {
		os.MkdirAll(d, 0o700)
	}
	hostHome, _ := os.UserHomeDir()
	fake := filepath.Join(work, "fake-agent")
	script := "#!/bin/sh\necho \"ARGS $*\"\necho \"HOME $HOME\"\nls " + hostHome + " >/dev/null 2>&1 && echo HOST-HOME-VISIBLE\ncat \"$HOME/.claude/.credentials.json\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	spec := isolate.Spec{Work: work, Home: home, Credentials: isolate.CredentialsPath(), Env: isolate.BaseEnv(home)}
	res, err := IsolatedSpawner{Binary: fake}.Spawn(context.Background(), SpawnRequest{Prompt: "hello", WorkDir: work, Env: spec.Env, Isolate: &spec, ExtraArgs: []string{"--x"}})
	if err != nil {
		t.Fatalf("Spawn: %v\n%s", err, res.Stdout)
	}
	for _, want := range []string{"ARGS -p --permission-mode bypassPermissions --setting-sources project --x -- hello", "HOME " + home, "at-test"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, res.Stdout)
		}
	}
	if strings.Contains(res.Stdout, "HOST-HOME-VISIBLE") || strings.Contains(res.Stdout, "rt-test") {
		t.Errorf("host home visible or refresh token copied:\n%s", res.Stdout)
	}
}

func TestIsolatedSpawnerRefusesNoSpec(t *testing.T) {
	if _, err := (IsolatedSpawner{}).Spawn(context.Background(), SpawnRequest{Prompt: "x"}); err == nil {
		t.Error("IsolatedSpawner ran with no sandbox spec")
	}
}

func TestProbeIsolatedSinksBlockedHosts(t *testing.T) {
	requireBwrap(t)
	longLivedCreds(t)
	o := JudgeOptions{Isolation: "bwrap", SandboxParent: t.TempDir()}
	out, err := ProbeIsolated(context.Background(), o, []string{"getent", "ahosts", "api.notion.com"})
	if err != nil || firstField(out) != "0.0.0.0" {
		t.Errorf("probe = %q, %v; want 0.0.0.0", out, err)
	}
	hostHome, _ := os.UserHomeDir()
	if out, err := ProbeIsolated(context.Background(), o, []string{"ls", hostHome}); err == nil {
		t.Errorf("probe sees the host home: %q", out)
	}
}

func callFake(t *testing.T, url, tool string, args map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Errorf("call fake %s: %v", url, err)
		return
	}
	resp.Body.Close()
}

func TestProbeIsolatedRemovesItsSandbox(t *testing.T) {
	requireBwrap(t)
	longLivedCreds(t)
	parent := t.TempDir()
	if _, err := ProbeIsolated(context.Background(), JudgeOptions{Isolation: "bwrap", SandboxParent: parent}, []string{"true"}); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(parent); len(left) != 0 {
		t.Errorf("probe left %d entries in %s", len(left), parent)
	}
}
