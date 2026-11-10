package evalharness

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/acceptpending"
	"github.com/versality/spore/internal/research/fakemcp"
)

// TestLiveAcceptance08FakeNotionCallRecorded spends real API budget
// once Phase C lands:
//
//	EVALHARNESS_LIVE=1 go test ./internal/evalharness/ -run TestLiveAcceptance08 -v -timeout 10m
func TestLiveAcceptance08FakeNotionCallRecorded(t *testing.T) {
	acceptpending.Skip(t, "C")
	if os.Getenv("EVALHARNESS_LIVE") == "" {
		t.Skip("set EVALHARNESS_LIVE=1 to run a real, sandboxed agent under test")
	}
	opts := JudgeOptions{
		SetDir:        "testdata/notion-set/main",
		Candidate:     "testdata/notion-set/skill/SKILL.md",
		FrozenDir:     "testdata/notion-set/skill",
		SkillName:     "notion-demo",
		Isolation:     "bwrap",
		SandboxParent: t.TempDir(),
		Repeats:       1,
		Parallel:      1,
		Timeout:       5 * time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	rep, err := RunJudge(ctx, opts)
	if err != nil {
		t.Errorf("RunJudge: %v", err)
	}
	if rep.Score != 100 {
		t.Errorf("score = %v, want 100", rep.Score)
	}

	if out, err := ProbeIsolated(ctx, opts, []string{"getent", "hosts", "api.notion.com"}); err != nil || firstField(out) != "0.0.0.0" {
		t.Errorf("isolated getent hosts api.notion.com = %q, %v; want it to resolve to 0.0.0.0", out, err)
	}

	if len(rep.Scenarios) != 1 || len(rep.Scenarios[0].Runs) != 1 {
		t.Errorf("report scenarios = %+v, want one scenario with one run", rep.Scenarios)
		return
	}
	run := rep.Scenarios[0].Runs[0]

	calls, err := fakemcp.ReadCalls(run.CallsLog)
	if err != nil {
		t.Errorf("ReadCalls(%q): %v", run.CallsLog, err)
	}
	if n := countTool(calls, "notion-create-pages"); n != 1 {
		t.Errorf("recorded notion-create-pages calls = %d, want exactly 1 (all calls: %+v)", n, calls)
	}

	transcript, err := os.ReadFile(run.TranscriptPath)
	if err != nil {
		t.Errorf("read transcript: %v", err)
	}
	servers, tools, ok := initEvent(string(transcript))
	if !ok {
		t.Errorf("transcript has no system/init event")
		return
	}
	if strings.Join(servers, ",") != "claude_ai_Notion" {
		t.Errorf("init MCP servers = %v, want exactly [claude_ai_Notion]", servers)
	}
	for _, tool := range tools {
		if strings.HasPrefix(tool, "mcp__") && !strings.HasPrefix(tool, "mcp__claude_ai_Notion__") {
			t.Errorf("init lists MCP tool %q from outside the fake Notion server", tool)
		}
	}
}

func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func countTool(calls []fakemcp.Call, tool string) int {
	n := 0
	for _, c := range calls {
		if c.Tool == tool {
			n++
		}
	}
	return n
}

// initEvent returns the sorted MCP server names and the tool names
// from a stream-json transcript's system/init line.
func initEvent(transcript string) (servers, tools []string, ok bool) {
	for _, line := range strings.Split(transcript, "\n") {
		var ev struct {
			Type       string   `json:"type"`
			Subtype    string   `json:"subtype"`
			Tools      []string `json:"tools"`
			MCPServers []struct {
				Name string `json:"name"`
			} `json:"mcp_servers"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "system" || ev.Subtype != "init" {
			continue
		}
		for _, s := range ev.MCPServers {
			servers = append(servers, s.Name)
		}
		sort.Strings(servers)
		return servers, ev.Tools, true
	}
	return nil, nil, false
}
