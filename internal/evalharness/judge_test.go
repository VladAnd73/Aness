package evalharness

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versality/spore/internal/research/fakemcp"
)

func resultLine(reply string, cost float64) string {
	return fmt.Sprintf(`{"type":"result","result":%q,"total_cost_usd":%v,"usage":{"input_tokens":10,"output_tokens":20}}`, reply, cost)
}

// scriptSpawner answers each spawn with fn and records every request.
type scriptSpawner struct {
	mu       sync.Mutex
	reqs     []SpawnRequest
	active   int
	maxAlive int
	fn       func(ctx context.Context, req SpawnRequest) (SpawnResult, error)
}

func (s *scriptSpawner) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.active++
	s.maxAlive = max(s.maxAlive, s.active)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	return s.fn(ctx, req)
}

func replyOK(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	reply := "nope"
	if strings.HasPrefix(req.Prompt, "pass") {
		reply = "OK"
	}
	return SpawnResult{Stdout: resultLine(reply, 0.5)}, nil
}

// writeSet makes a set with one scenario per prompt; each passes when
// the final reply contains OK.
func writeSet(t *testing.T, prompts ...string) string {
	t.Helper()
	set := t.TempDir()
	for i, p := range prompts {
		dir := filepath.Join(set, fmt.Sprintf("s%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		sc := fmt.Sprintf(`{"prompt":%q,"checks":[{"kind":"reply-contains","text":"OK"}]}`, p)
		if err := os.WriteFile(filepath.Join(dir, "scenario.json"), []byte(sc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return set
}

func judgeOpts(t *testing.T, set string, sp AgentSpawner) JudgeOptions {
	t.Helper()
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "1")
	cand := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(cand, []byte("---\nname: demo\n---\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return JudgeOptions{
		SetDir: set, Candidate: cand, SkillName: "demo", Isolation: "none",
		SandboxParent: t.TempDir(), Spawner: sp,
	}
}

func firstRun(t *testing.T, rep JudgeReport) RunReport {
	t.Helper()
	if len(rep.Scenarios) == 0 || len(rep.Scenarios[0].Runs) == 0 {
		t.Fatalf("report has no runs: %+v", rep)
	}
	return rep.Scenarios[0].Runs[0]
}

func TestInjectSkillPlacesCandidateOverFrozenFolder(t *testing.T) {
	frozen := t.TempDir()
	os.WriteFile(filepath.Join(frozen, "SKILL.md"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(frozen, "refs"), 0o755)
	os.WriteFile(filepath.Join(frozen, "refs", "notes.md"), []byte("sibling"), 0o644)
	cand := filepath.Join(t.TempDir(), "cand.md")
	os.WriteFile(cand, []byte("new body"), 0o644)
	wd := t.TempDir()

	if err := InjectSkill(wd, frozen, "demo", cand); err != nil {
		t.Fatalf("InjectSkill: %v", err)
	}
	dst := filepath.Join(wd, ".claude", "skills", "demo")
	if b, _ := os.ReadFile(filepath.Join(dst, "SKILL.md")); string(b) != "new body" {
		t.Errorf("SKILL.md = %q, want the candidate", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "refs", "notes.md")); string(b) != "sibling" {
		t.Errorf("frozen sibling = %q", b)
	}
}

func TestInjectSkillRejectsAPathAsName(t *testing.T) {
	cand := filepath.Join(t.TempDir(), "cand.md")
	os.WriteFile(cand, []byte("x"), 0o644)
	for _, name := range []string{"", "../x", "a/b"} {
		if err := InjectSkill(t.TempDir(), "", name, cand); err == nil {
			t.Errorf("InjectSkill accepted skill name %q", name)
		}
	}
}

func TestRunJudgeScoresPercentOfRunsPassed(t *testing.T) {
	sp := &scriptSpawner{fn: replyOK}
	o := judgeOpts(t, writeSet(t, "pass a", "pass b", "fail c"), sp)
	o.Repeats = 3

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if rep.Runs != 9 || rep.Passed != 6 || rep.Crashes != 0 {
		t.Errorf("runs/passed/crashes = %d/%d/%d, want 9/6/0", rep.Runs, rep.Passed, rep.Crashes)
	}
	if math.Abs(rep.Score-200.0/3) > 1e-9 {
		t.Errorf("score = %v, want 66.666...", rep.Score)
	}
	if len(rep.Scenarios) != 3 || rep.Scenarios[0].Name != "s0" || len(rep.Scenarios[2].Runs) != 3 {
		t.Fatalf("scenarios = %+v", rep.Scenarios)
	}
	if r := rep.Scenarios[2].Runs[0]; r.Pass || r.Reason != "check 0 (reply-contains)" {
		t.Errorf("failing run = %+v, want reason naming the check by index and kind", r)
	}
}

func TestRunJudgeCountsTimeoutAsCrash(t *testing.T) {
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		<-ctx.Done()
		return SpawnResult{ExitCode: -1}, ctx.Err()
	}}
	o := judgeOpts(t, writeSet(t, "pass a", "pass b"), sp)
	o.Repeats, o.Timeout = 1, 50*time.Millisecond

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if rep.Crashes != rep.Runs || rep.Runs != 2 || rep.Score != 0 {
		t.Errorf("report = %+v, want every run crashed and score 0", rep)
	}
	if r := firstRun(t, rep); !r.Crashed || r.Reason != "timeout" {
		t.Errorf("run = %+v, want crashed with reason timeout", r)
	}
}

func TestRunJudgeCountsBadExitAndMissingResultAsCrash(t *testing.T) {
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		if strings.Contains(req.Prompt, "exit") {
			return SpawnResult{Stdout: resultLine("OK", 0.1), ExitCode: 3}, fmt.Errorf("exit status 3")
		}
		return SpawnResult{Stdout: `{"type":"assistant","message":{"content":[]}}`}, nil
	}}
	o := judgeOpts(t, writeSet(t, "pass exit", "pass noresult"), sp)
	o.Repeats = 1

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if rep.Crashes != 2 || rep.Passed != 0 {
		t.Errorf("report = %+v, want 2 crashes", rep)
	}
}

func TestRunJudgeHonoursParallelLimit(t *testing.T) {
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		time.Sleep(30 * time.Millisecond)
		return replyOK(ctx, req)
	}}
	o := judgeOpts(t, writeSet(t, "pass a", "pass b", "pass c"), sp)
	o.Repeats, o.Parallel = 3, 3

	if _, err := RunJudge(context.Background(), o); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if len(sp.reqs) != 9 || sp.maxAlive != 3 {
		t.Errorf("spawns = %d, max concurrent = %d; want 9 and 3", len(sp.reqs), sp.maxAlive)
	}
}

func TestRunJudgeRepeatsDoNotShareASandbox(t *testing.T) {
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		if _, err := os.Stat(filepath.Join(req.WorkDir, "mark")); err == nil {
			return SpawnResult{Stdout: resultLine("shared", 0)}, nil
		}
		os.WriteFile(filepath.Join(req.WorkDir, "mark"), nil, 0o644)
		return replyOK(ctx, req)
	}}
	o := judgeOpts(t, writeSet(t, "pass a"), sp)
	o.Repeats = 3

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	dirs := map[string]bool{}
	for _, r := range sp.reqs {
		dirs[r.WorkDir] = true
	}
	if rep.Passed != 3 || len(dirs) != 3 {
		t.Errorf("passed = %d, distinct work dirs = %d; want 3 and 3", rep.Passed, len(dirs))
	}
}

func TestRunJudgePassesModel(t *testing.T) {
	for _, model := range []string{"opus", ""} {
		sp := &scriptSpawner{fn: replyOK}
		o := judgeOpts(t, writeSet(t, "pass a"), sp)
		o.Repeats, o.Model = 1, model
		if _, err := RunJudge(context.Background(), o); err != nil {
			t.Fatalf("RunJudge: %v", err)
		}
		if len(sp.reqs) == 0 {
			t.Fatal("no spawn")
		}
		args := sp.reqs[0].ExtraArgs
		i := slices.Index(args, "--model")
		if model == "" && i >= 0 {
			t.Errorf("args = %v, want no --model when Model is empty", args)
		}
		if model != "" && (i < 0 || i+1 >= len(args) || args[i+1] != model) {
			t.Errorf("args = %v, want --model %s", args, model)
		}
		if !slices.Contains(args, "--strict-mcp-config") || !slices.Contains(args, "stream-json") {
			t.Errorf("args = %v, want stream-json output and --strict-mcp-config", args)
		}
	}
}

func TestRunJudgeSumsCost(t *testing.T) {
	sp := &scriptSpawner{fn: replyOK}
	o := judgeOpts(t, writeSet(t, "pass a"), sp)
	o.Repeats = 3

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if math.Abs(rep.CostUSD-1.5) > 1e-9 || rep.InputTokens != 30 || rep.OutputTokens != 60 {
		t.Errorf("cost/in/out = %v/%d/%d, want 1.5/30/60", rep.CostUSD, rep.InputTokens, rep.OutputTokens)
	}
}

func TestRunJudgeInjectsCandidateAndScenarioFiles(t *testing.T) {
	set := writeSet(t, "pass a")
	os.MkdirAll(filepath.Join(set, "s0", "files", "sub"), 0o755)
	os.WriteFile(filepath.Join(set, "s0", "files", "sub", "seed.txt"), []byte("seed"), 0o644)
	var sawSkill, sawSeed string
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		b, _ := os.ReadFile(filepath.Join(req.WorkDir, ".claude", "skills", "demo", "SKILL.md"))
		sawSkill = string(b)
		b, _ = os.ReadFile(filepath.Join(req.WorkDir, "sub", "seed.txt"))
		sawSeed = string(b)
		return replyOK(ctx, req)
	}}
	o := judgeOpts(t, set, sp)
	o.Repeats = 1
	if _, err := RunJudge(context.Background(), o); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if !strings.Contains(sawSkill, "candidate") || sawSeed != "seed" {
		t.Errorf("agent saw skill %q and seed %q", sawSkill, sawSeed)
	}
}

func TestRunJudgeKeepsTranscriptInTheReport(t *testing.T) {
	sp := &scriptSpawner{fn: replyOK}
	o := judgeOpts(t, writeSet(t, "pass a"), sp)
	o.Repeats = 1
	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	run := firstRun(t, rep)
	if b, err := os.ReadFile(run.TranscriptPath); err != nil || !strings.Contains(string(b), `"result":"OK"`) {
		t.Errorf("transcript %q = %q, %v", run.TranscriptPath, b, err)
	}
	if run.SandboxRoot == "" || !strings.HasPrefix(run.TranscriptPath, run.SandboxRoot) {
		t.Errorf("run = %+v, want the transcript inside the run's sandbox", run)
	}
}

func TestRunJudgeRefusesUnsandboxedWithoutOptIn(t *testing.T) {
	sp := &scriptSpawner{fn: replyOK}
	o := judgeOpts(t, writeSet(t, "pass a"), sp)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "")
	if _, err := RunJudge(context.Background(), o); err == nil || !strings.Contains(err.Error(), "SPORE_RESEARCH_ALLOW_UNSANDBOXED") {
		t.Errorf("err = %v, want a refusal naming the opt-in variable", err)
	}
	if len(sp.reqs) != 0 {
		t.Errorf("spawned %d agents before refusing", len(sp.reqs))
	}
}

func TestRunJudgeWiresFakeServicesAndGradesTheirCalls(t *testing.T) {
	var gotTools []fakemcp.Tool
	stopped := 0
	starter := func(ctx context.Context, svcs []fakemcp.Tool, callsLog string) ([]byte, func() error, error) {
		gotTools = svcs
		line, _ := json.Marshal(fakemcp.Call{Server: "claude_ai_Notion", Tool: "notion-create-pages", Args: map[string]any{"title": "Nightly"}})
		if err := os.WriteFile(callsLog, append(line, '\n'), 0o600); err != nil {
			return nil, nil, err
		}
		return []byte(`{"mcpServers":{}}`), func() error { stopped++; return nil }, nil
	}
	var mcpConfig string
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		if i := slices.Index(req.ExtraArgs, "--mcp-config"); i >= 0 && i+1 < len(req.ExtraArgs) {
			b, _ := os.ReadFile(req.ExtraArgs[i+1])
			mcpConfig = string(b)
		}
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	o := judgeOpts(t, "testdata/notion-set/main", sp)
	o.Repeats, o.Services = 1, starter

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if rep.Passed != 1 || len(gotTools) != 1 || stopped != 1 || mcpConfig != `{"mcpServers":{}}` {
		t.Errorf("passed=%d tools=%v stopped=%d mcp=%q", rep.Passed, gotTools, stopped, mcpConfig)
	}
	if firstRun(t, rep).CallsLog == "" {
		t.Error("report does not carry the calls log path")
	}
}

func TestFixtureFilesCannotOverwriteCandidate(t *testing.T) {
	set := writeSet(t, "pass a")
	fixture := filepath.Join(set, "s0", "files", ".claude", "skills", "demo")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "SKILL.md"), []byte("FIXTURE"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen string
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		b, _ := os.ReadFile(filepath.Join(req.WorkDir, ".claude", "skills", "demo", "SKILL.md"))
		seen = string(b)
		return replyOK(ctx, req)
	}}
	if _, err := RunJudge(context.Background(), judgeOpts(t, set, sp)); err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if !strings.Contains(seen, "candidate") {
		t.Errorf("agent under test saw SKILL.md %q, want the candidate (fixture files must not win)", seen)
	}
}

func TestRunJudgeFakeServiceFailureIsCrash(t *testing.T) {
	starter := func(ctx context.Context, svcs []fakemcp.Tool, callsLog string) ([]byte, func() error, error) {
		line, _ := json.Marshal(fakemcp.Call{Server: "claude_ai_Notion", Tool: "notion-create-pages", Args: map[string]any{"title": "Nightly"}})
		if err := os.WriteFile(callsLog, append(line, '\n'), 0o600); err != nil {
			return nil, nil, err
		}
		return []byte(`{"mcpServers":{}}`), func() error { return fmt.Errorf("serve broke") }, nil
	}
	sp := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		return SpawnResult{Stdout: resultLine("done", 0)}, nil
	}}
	o := judgeOpts(t, "testdata/notion-set/main", sp)
	o.Repeats, o.Services = 1, starter
	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if run := firstRun(t, rep); run.Pass || !run.Crashed {
		t.Errorf("fake services failed mid-run: run = %+v, want a crash", run)
	}
}
