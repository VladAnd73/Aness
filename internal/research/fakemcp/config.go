package fakemcp

import "encoding/json"

// MCPConfig renders servers (name -> URL) as a claude --mcp-config file.
func MCPConfig(servers map[string]string) []byte {
	type entry struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	cfg := struct {
		MCPServers map[string]entry `json:"mcpServers"`
	}{MCPServers: map[string]entry{}}
	for name, url := range servers {
		cfg.MCPServers[name] = entry{Type: "http", URL: url}
	}
	b, _ := json.Marshal(cfg)
	return b
}
