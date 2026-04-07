package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MCPServerEntry describes a single MCP server in a config file.
type MCPServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// ClaudeCodeMCPConfig is the structure of .claude/mcp.json.
type ClaudeCodeMCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// CopilotMCPConfig is the structure of ~/.copilot/mcp-config.json.
type CopilotMCPConfig struct {
	Servers map[string]MCPServerEntry `json:"servers"`
}

// WriteMCPConfig merges a server entry into a Claude Code-format MCP config file.
// Creates the file and parent directories if they don't exist.
func WriteMCPConfig(path, serverName string, entry MCPServerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var config ClaudeCodeMCPConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &config)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPServerEntry)
	}
	config.MCPServers[serverName] = entry

	return writeJSON(path, config)
}

// WriteCopilotMCPConfig merges a server entry into a Copilot-format MCP config file.
func WriteCopilotMCPConfig(path, serverName string, entry MCPServerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var config CopilotMCPConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &config)
	}
	if config.Servers == nil {
		config.Servers = make(map[string]MCPServerEntry)
	}
	config.Servers[serverName] = entry

	return writeJSON(path, config)
}

// RemoveMCPConfig removes a server entry from a Claude Code-format MCP config.
func RemoveMCPConfig(path, serverName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var config ClaudeCodeMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	delete(config.MCPServers, serverName)
	return writeJSON(path, config)
}

// RemoveCopilotMCPConfig removes a server entry from a Copilot-format MCP config.
func RemoveCopilotMCPConfig(path, serverName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var config CopilotMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	delete(config.Servers, serverName)
	return writeJSON(path, config)
}

// BackupConfig copies the file at path to path.ratchet-bak and returns the backup path.
// Returns ("", nil) if the file does not exist.
func BackupConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backupPath := path + ".ratchet-bak"
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", err
	}
	return backupPath, nil
}

// RestoreConfig restores the backup to the original path and removes the backup.
func RestoreConfig(path, backupPath string) error {
	if backupPath == "" {
		// No backup means the file didn't exist before — remove it.
		os.Remove(path)
		return nil
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return os.Remove(backupPath)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
