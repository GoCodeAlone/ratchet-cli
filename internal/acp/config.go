package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ZedACPAgentServer describes one custom ACP agent server in Zed settings.
type ZedACPAgentServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// ZedACPConfig is the ACP-related subset of Zed settings.
type ZedACPConfig struct {
	AgentServers map[string]ZedACPAgentServer `json:"agent_servers"`
}

// WriteZedACPConfig merges a custom ACP agent into a Zed settings.json file.
func WriteZedACPConfig(path, serverName string, entry ZedACPAgentServer) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	servers := map[string]ZedACPAgentServer{}
	if data, ok := raw["agent_servers"]; ok {
		if err := json.Unmarshal(data, &servers); err != nil {
			return fmt.Errorf("parse agent_servers: %w", err)
		}
	}
	if entry.Env == nil {
		entry.Env = map[string]string{}
	}
	servers[serverName] = entry

	data, err := json.Marshal(servers)
	if err != nil {
		return fmt.Errorf("encode agent_servers: %w", err)
	}
	raw["agent_servers"] = data

	return writeJSON(path, raw)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
