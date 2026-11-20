package evalharness

import (
	"os"
	"strings"
	"testing"
)

// sample.jsonl is a real `claude -p --model haiku --output-format
// stream-json --verbose --setting-sources project --strict-mcp-config`
// run (2026-11-10, CLI 2.1.296) with host paths scrubbed.
func TestParseStream(t *testing.T) {
	raw, err := os.ReadFile("testdata/stream/sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	got := ParseStream(string(raw) + `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Trunc`)

	if !got.SawResult {
		t.Error("SawResult = false")
	}
	if got.FinalReply != "PROBE-DONE" {
		t.Errorf("FinalReply = %q", got.FinalReply)
	}
	if got.CostUSD < 0.002 || got.CostUSD > 0.0022 {
		t.Errorf("CostUSD = %v, want 0.00217...", got.CostUSD)
	}
	if got.InputTokens != 6 || got.OutputTokens != 336 {
		t.Errorf("tokens = %d in, %d out; want 6, 336", got.InputTokens, got.OutputTokens)
	}
	if strings.Join(got.ToolsUsed, ",") != "Skill,Write" {
		t.Errorf("ToolsUsed = %v, want [Skill Write]", got.ToolsUsed)
	}
	if len(got.MCPServers) != 0 {
		t.Errorf("MCPServers = %v, want none under --strict-mcp-config", got.MCPServers)
	}
}

func TestParseStreamReadsMCPServerNames(t *testing.T) {
	got := ParseStream(`{"type":"system","subtype":"init","mcp_servers":[{"name":"claude_ai_Notion","status":"connected"}]}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__claude_ai_Notion__notion-create-pages"}]}}`)
	if strings.Join(got.MCPServers, ",") != "claude_ai_Notion" {
		t.Errorf("MCPServers = %v", got.MCPServers)
	}
	if strings.Join(got.ToolsUsed, ",") != "mcp__claude_ai_Notion__notion-create-pages" {
		t.Errorf("ToolsUsed = %v", got.ToolsUsed)
	}
	if got.SawResult {
		t.Error("SawResult = true with no result line")
	}
}
