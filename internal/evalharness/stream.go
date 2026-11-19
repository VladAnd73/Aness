package evalharness

import (
	"bufio"
	"encoding/json"
	"slices"
	"strings"
)

// StreamResult is what grading and scoring read from one run's
// `claude -p --output-format stream-json --verbose` stdout. Token
// counts are the result line's usage.input_tokens and output_tokens,
// which exclude cache reads and cache writes.
type StreamResult struct {
	FinalReply                string
	ToolsUsed                 []string
	MCPServers                []string
	CostUSD                   float64
	InputTokens, OutputTokens int64
	SawResult                 bool
}

type streamLine struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	MCPServers []struct {
		Name string `json:"name"`
	} `json:"mcp_servers"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	} `json:"message"`
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// ParseStream reads a stream-json transcript line by line. A line that
// is not JSON (a killed run's partial last write, or the stderr block
// execCommand appends) is skipped.
func ParseStream(stdout string) StreamResult {
	var r StreamResult
	sc := bufio.NewScanner(strings.NewReader(stdout))
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var l streamLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch {
		case l.Type == "system" && l.Subtype == "init":
			for _, s := range l.MCPServers {
				r.MCPServers = append(r.MCPServers, s.Name)
			}
		case l.Type == "assistant":
			for _, b := range l.Message.Content {
				if b.Type == "tool_use" && !slices.Contains(r.ToolsUsed, b.Name) {
					r.ToolsUsed = append(r.ToolsUsed, b.Name)
				}
			}
		case l.Type == "result":
			r.SawResult = true
			r.FinalReply = l.Result
			r.CostUSD = l.TotalCostUSD
			r.InputTokens = l.Usage.InputTokens
			r.OutputTokens = l.Usage.OutputTokens
		}
	}
	return r
}
