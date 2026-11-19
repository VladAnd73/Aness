package fakemcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Acceptance 5: a keyed tool serves only the ids the scenario allows;
// any other id is not found, so the agent cannot reach a bug ticket.
func TestAcceptanceRole5KeyedToolServesOnlyAllowedIDs(t *testing.T) {
	tools := []Tool{{
		Server: "claude_ai_Linear", Name: "get_issue", Key: "id",
		Responses: map[string]json.RawMessage{"FAKE-1": json.RawMessage(`{"id":"FAKE-1","title":"Rename a listing"}`)},
	}}
	urls, stop, err := Start(context.Background(), tools, filepath.Join(t.TempDir(), "calls.jsonl"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stop() })
	call := func(id string) (text string, isError bool) {
		r := rpc(t, urls["claude_ai_Linear"], 1, "tools/call", map[string]any{"name": "get_issue", "arguments": map[string]any{"id": id}})
		res, _ := r.body["result"].(map[string]any)
		content, _ := res["content"].([]any)
		for _, c := range content {
			m, _ := c.(map[string]any)
			text += fmt.Sprint(m["text"])
		}
		isError, _ = res["isError"].(bool)
		return text, isError
	}
	if text, isErr := call("FAKE-1"); isErr || !strings.Contains(text, "Rename a listing") {
		t.Errorf("allowed id: text %q isError %v", text, isErr)
	}
	if text, isErr := call("FAKE-2"); !isErr || !strings.Contains(text, "not found") || strings.Contains(text, "Rename") {
		t.Errorf("id outside the allowed list: text %q isError %v, want a not-found error", text, isErr)
	}
}
