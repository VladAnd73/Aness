package fakemcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var notionTools = []Tool{
	{Server: "claude_ai_Notion", Name: "notion-create-pages", Response: json.RawMessage(`{"pages":[{"id":"page-1"}]}`)},
	{Server: "claude_ai_Notion", Name: "notion-search", Response: json.RawMessage(`{"results":[]}`)},
	{Server: "claude_ai_Linear", Name: "create_issue", Response: json.RawMessage(`{"id":"L-1"}`)},
}

func start(t *testing.T) (map[string]string, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	urls, stop, err := Start(context.Background(), notionTools, log)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stop() })
	return urls, log
}

type rpcReply struct {
	status int
	body   map[string]any
}

func rpc(t *testing.T, url string, id any, method string, params any) rpcReply {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id != nil {
		msg["id"] = id
	}
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("POST %s: %v", url, err)
		return rpcReply{}
	}
	defer resp.Body.Close()
	r := rpcReply{status: resp.StatusCode}
	_ = json.NewDecoder(resp.Body).Decode(&r.body)
	return r
}

func toolNames(r rpcReply) []string {
	res, _ := r.body["result"].(map[string]any)
	list, _ := res["tools"].([]any)
	var out []string
	for _, x := range list {
		m, _ := x.(map[string]any)
		out = append(out, fmt.Sprint(m["name"]))
	}
	return out
}

func TestFakeMCPListsDeclaredTools(t *testing.T) {
	urls, _ := start(t)
	if len(urls) != 2 || urls["claude_ai_Notion"] == "" || urls["claude_ai_Linear"] == "" {
		t.Fatalf("urls = %v, want one per server", urls)
	}
	got := toolNames(rpc(t, urls["claude_ai_Notion"], 1, "tools/list", map[string]any{}))
	if fmt.Sprint(got) != "[notion-create-pages notion-search]" {
		t.Errorf("Notion tools = %v", got)
	}
	got = toolNames(rpc(t, urls["claude_ai_Linear"], 1, "tools/list", map[string]any{}))
	if fmt.Sprint(got) != "[create_issue]" {
		t.Errorf("Linear tools = %v", got)
	}
}

func TestFakeMCPInitializeEchoesVersion(t *testing.T) {
	urls, _ := start(t)
	r := rpc(t, urls["claude_ai_Notion"], 1, "initialize", map[string]any{"protocolVersion": "2099-01-01"})
	res, _ := r.body["result"].(map[string]any)
	caps, _ := res["capabilities"].(map[string]any)
	if r.status != 200 || res["protocolVersion"] != "2099-01-01" || caps["tools"] == nil {
		t.Errorf("initialize = %d %v", r.status, r.body)
	}
	if r := rpc(t, urls["claude_ai_Notion"], nil, "notifications/initialized", nil); r.status != http.StatusAccepted {
		t.Errorf("notification status = %d, want 202", r.status)
	}
	if r := rpc(t, urls["claude_ai_Notion"], 2, "ping", nil); r.status != 200 || r.body["result"] == nil {
		t.Errorf("ping = %d %v", r.status, r.body)
	}
}

// claude opens a GET for server-sent events; any answer but 405 makes
// it poll forever and never exit (measured live, step zero).
func TestFakeMCPRefusesEventStreamGet(t *testing.T) {
	urls, _ := start(t)
	resp, err := http.Get(urls["claude_ai_Notion"])
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
}

func TestFakeMCPCallReturnsCannedResponseAndLogs(t *testing.T) {
	urls, log := start(t)
	r := rpc(t, urls["claude_ai_Notion"], 7, "tools/call", map[string]any{"name": "notion-create-pages", "arguments": map[string]any{"title": "Nightly", "n": 2}})
	res, _ := r.body["result"].(map[string]any)
	content, _ := res["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("tools/call = %v", r.body)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if text != `{"pages":[{"id":"page-1"}]}` || res["isError"] == true {
		t.Errorf("content text = %q (%v)", text, res)
	}
	calls, err := ReadCalls(log)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Server != "claude_ai_Notion" || calls[0].Tool != "notion-create-pages" ||
		calls[0].Args["title"] != "Nightly" || calls[0].Args["n"] != float64(2) || calls[0].Error != "" || calls[0].At.IsZero() {
		t.Errorf("calls = %+v", calls)
	}
}

func TestFakeMCPUnknownToolErrorsAndLogs(t *testing.T) {
	urls, log := start(t)
	r := rpc(t, urls["claude_ai_Linear"], 3, "tools/call", map[string]any{"name": "notion-search", "arguments": map[string]any{}})
	e, _ := r.body["error"].(map[string]any)
	if e["code"] != float64(-32601) {
		t.Errorf("unknown tool reply = %v, want error -32601", r.body)
	}
	calls, _ := ReadCalls(log)
	if len(calls) != 1 || calls[0].Error == "" || calls[0].Server != "claude_ai_Linear" {
		t.Errorf("calls = %+v, want one logged error", calls)
	}
}

func TestFakeMCPConcurrentCallsAllLogged(t *testing.T) {
	urls, log := start(t)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rpc(t, urls["claude_ai_Notion"], i, "tools/call", map[string]any{"name": "notion-search", "arguments": map[string]any{"i": i, "pad": string(bytes.Repeat([]byte("x"), 4096))}})
		}()
	}
	wg.Wait()
	b, _ := os.ReadFile(log)
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	if len(lines) != 50 {
		t.Fatalf("log has %d lines, want 50", len(lines))
	}
	for i, l := range lines {
		if !json.Valid(l) {
			t.Errorf("line %d is not valid JSON", i+1)
		}
	}
}

func TestFakeMCPStopClosesListener(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	urls, stop, err := Start(context.Background(), notionTools, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := http.Get(urls["claude_ai_Notion"]); err == nil {
		t.Error("server still answers after stop")
	}
}

func TestMCPConfigShape(t *testing.T) {
	got := string(MCPConfig(map[string]string{"b": "http://127.0.0.1:1/mcp/b", "a": "http://127.0.0.1:1/mcp/a"}))
	want := `{"mcpServers":{"a":{"type":"http","url":"http://127.0.0.1:1/mcp/a"},"b":{"type":"http","url":"http://127.0.0.1:1/mcp/b"}}}`
	if got != want {
		t.Errorf("MCPConfig =\n%s\nwant\n%s", got, want)
	}
	if got := string(MCPConfig(nil)); got != `{"mcpServers":{}}` {
		t.Errorf("empty MCPConfig = %s", got)
	}
}

func TestServeReadsServicesFile(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "services.json")
	b, _ := json.Marshal(notionTools)
	os.WriteFile(svc, b, 0o600)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, svc, filepath.Join(dir, "calls.jsonl"), "127.0.0.1:0") }()
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Serve after cancel = %v, want nil", err)
	}
	if err := Serve(context.Background(), filepath.Join(dir, "absent.json"), filepath.Join(dir, "c.jsonl"), "127.0.0.1:0"); err == nil {
		t.Error("Serve accepted a missing services file")
	}
}

func TestFakeMCPListsDeclaredInputSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}`
	tools := []Tool{{Server: "s", Name: "a", InputSchema: json.RawMessage(schema), Response: json.RawMessage(`{}`)}}
	urls, stop, err := Start(context.Background(), tools, filepath.Join(t.TempDir(), "c.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	r := rpc(t, urls["s"], 1, "tools/list", nil)
	list, _ := r.body["result"].(map[string]any)["tools"].([]any)
	if len(list) != 1 {
		t.Fatalf("tools/list = %v", r.body)
	}
	got, _ := json.Marshal(list[0].(map[string]any)["inputSchema"])
	var want any
	_ = json.Unmarshal([]byte(schema), &want)
	if w, _ := json.Marshal(want); string(got) != string(w) {
		t.Errorf("inputSchema = %s, want %s", got, schema)
	}
}
