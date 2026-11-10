package fakemcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCallsReadsJSONLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "calls.jsonl")
	os.WriteFile(p, []byte(`{"server":"s","tool":"a","args":{"x":"1"},"at":"2026-11-10T00:00:00Z"}`+"\n"+`{"server":"s","tool":"b","args":{},"error":"unknown tool","at":"2026-11-10T00:00:01Z"}`+"\n"), 0o600)
	calls, err := ReadCalls(p)
	if err != nil {
		t.Fatalf("ReadCalls: %v", err)
	}
	if len(calls) != 2 || calls[0].Tool != "a" || calls[0].Args["x"] != "1" || calls[1].Error != "unknown tool" {
		t.Errorf("calls = %+v", calls)
	}
}

func TestReadCallsMissingLogIsNoCalls(t *testing.T) {
	calls, err := ReadCalls(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil || len(calls) != 0 {
		t.Errorf("ReadCalls = %v, %v; want no calls and no error", calls, err)
	}
}
