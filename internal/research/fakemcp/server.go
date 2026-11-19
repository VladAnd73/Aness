package fakemcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Start serves tools on one loopback listener, one path per distinct
// Server name (/mcp/<server>), and returns server name -> URL plus a
// function that stops the listener. It speaks MCP streamable HTTP with
// JSON responses only.
func Start(ctx context.Context, tools []Tool, callsLog string) (map[string]string, func() error, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("fakemcp: listen: %w", err)
	}
	return startOn(ln, tools, callsLog)
}

func startOn(ln net.Listener, tools []Tool, callsLog string) (map[string]string, func() error, error) {
	srv := &http.Server{Handler: newHandler(tools, callsLog)}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	urls := map[string]string{}
	for _, t := range tools {
		urls[t.Server] = "http://" + ln.Addr().String() + "/mcp/" + t.Server
	}
	var once sync.Once
	var stopErr error
	stop := func() error {
		once.Do(func() {
			closeErr := srv.Close()
			if err := <-served; !errors.Is(err, http.ErrServerClosed) {
				stopErr = fmt.Errorf("fakemcp: serve: %w", err)
				return
			}
			stopErr = closeErr
		})
		return stopErr
	}
	return urls, stop, nil
}

// Serve runs the fake in the foreground on listen until ctx ends.
// servicesFile is a JSON array of Tool. ready, when set, gets the
// server name -> URL map once the listener is up.
func Serve(ctx context.Context, servicesFile, callsLog, listen string, ready func(urls map[string]string)) error {
	b, err := os.ReadFile(servicesFile)
	if err != nil {
		return fmt.Errorf("fakemcp: services: %w", err)
	}
	var tools []Tool
	if err := json.Unmarshal(b, &tools); err != nil {
		return fmt.Errorf("fakemcp: services %s: %w", servicesFile, err)
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("fakemcp: listen: %w", err)
	}
	urls, stop, err := startOn(ln, tools, callsLog)
	if err != nil {
		return err
	}
	if ready != nil {
		ready(urls)
	}
	<-ctx.Done()
	return stop()
}

type handler struct {
	tools    map[string][]Tool
	callsLog string
	mu       sync.Mutex
}

func newHandler(tools []Tool, callsLog string) *handler {
	h := &handler{tools: map[string][]Tool{}, callsLog: callsLog}
	for _, t := range tools {
		h.tools[t.Server] = append(h.tools[t.Server], t)
	}
	return h
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server, ok := strings.CutPrefix(r.URL.Path, "/mcp/")
	if _, known := h.tools[server]; !ok || !known {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "no event stream", http.StatusMethodNotAllowed)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(w, nil, nil, &rpcError{Code: -32700, Message: "parse error"})
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		reply(w, req.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": server, "version": "0"},
		}, nil)
	case "ping":
		reply(w, req.ID, map[string]any{}, nil)
	case "tools/list":
		var list []map[string]any
		for _, t := range h.tools[server] {
			var schema any = map[string]any{"type": "object", "additionalProperties": true}
			if len(t.InputSchema) > 0 {
				schema = t.InputSchema
			}
			list = append(list, map[string]any{
				"name":        t.Name,
				"description": t.Name,
				"inputSchema": schema,
			})
		}
		reply(w, req.ID, map[string]any{"tools": list}, nil)
	case "tools/call":
		h.call(w, server, req)
	default:
		reply(w, req.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method})
	}
}

func (h *handler) call(w http.ResponseWriter, server string, req rpcRequest) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &p)
	c := Call{Server: server, Tool: p.Name, Args: p.Arguments, At: time.Now().UTC()}
	var tool *Tool
	for i, t := range h.tools[server] {
		if t.Name == p.Name {
			tool = &h.tools[server][i]
		}
	}
	if tool == nil {
		c.Error = "unknown tool"
	}
	if err := h.log(c); err != nil {
		reply(w, req.ID, nil, &rpcError{Code: -32603, Message: "call log: " + err.Error()})
		return
	}
	if tool == nil {
		reply(w, req.ID, nil, &rpcError{Code: -32601, Message: "unknown tool: " + p.Name})
		return
	}
	reply(w, req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(tool.Response)}},
	}, nil)
}

func (h *handler) log(c Call) error {
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := os.OpenFile(h.callsLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func reply(w http.ResponseWriter, id json.RawMessage, result any, e *rpcError) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id}
	if e != nil {
		msg["error"] = e
	} else {
		msg["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}
