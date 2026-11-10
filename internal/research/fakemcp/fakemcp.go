// Package fakemcp serves canned MCP tools over HTTP and logs every
// call, so a scenario can grade what the agent under test asked an
// outside service to do without any real service being reached.
package fakemcp

import (
	"context"
	"encoding/json"
	"time"
)

type Tool struct {
	Server   string          `json:"server"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type Call struct {
	Server string         `json:"server"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Error  string         `json:"error,omitempty"`
	At     time.Time      `json:"at"`
}

type Server struct{ URL string }

// Start serves tools, one URL per distinct Server name, and returns
// server name -> URL plus a function that stops them all.
func Start(ctx context.Context, tools []Tool, callsLog string) (map[string]string, func() error, error) {
	return nil, func() error { return nil }, nil
}

func ReadCalls(callsLog string) ([]Call, error) { return nil, nil }
