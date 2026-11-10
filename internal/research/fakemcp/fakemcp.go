// Package fakemcp serves canned MCP tools over HTTP and logs every
// call, so a scenario can grade what the agent under test asked an
// outside service to do without any real service being reached.
package fakemcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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

// ReadCalls reads a calls log, one JSON Call per line. A log that does
// not exist means no call was made.
func ReadCalls(callsLog string) ([]Call, error) {
	b, err := os.ReadFile(callsLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var calls []Call
	for i, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c Call
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("fakemcp: %s line %d: %w", callsLog, i+1, err)
		}
		calls = append(calls, c)
	}
	return calls, nil
}
