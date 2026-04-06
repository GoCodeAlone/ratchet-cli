package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest represents a plugin's plugin.json manifest.
type Manifest struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Author       Author       `json:"author"`
	Capabilities Capabilities `json:"capabilities"`
}

// Author holds plugin author metadata.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Capabilities declares which capability directories/files a plugin provides.
type Capabilities struct {
	Skills   string `json:"skills,omitempty"`   // relative dir path
	Agents   string `json:"agents,omitempty"`   // relative dir path
	Commands string `json:"commands,omitempty"` // relative dir path
	Tools    string `json:"tools,omitempty"`    // relative dir path
	Hooks    string `json:"hooks,omitempty"`    // relative file path
	MCP      string `json:"mcp,omitempty"`      // relative file path
}

// manifestPaths lists manifest locations in preference order.
var manifestPaths = []string{
	filepath.Join(".ratchet-plugin", "plugin.json"),
	filepath.Join(".claude-plugin", "plugin.json"),
}

// LoadManifest reads a plugin manifest from pluginDir.
// It tries .ratchet-plugin/plugin.json first, then .claude-plugin/plugin.json.
func LoadManifest(pluginDir string) (*Manifest, error) {
	for _, rel := range manifestPaths {
		path := filepath.Join(pluginDir, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read manifest %s: %w", path, err)
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", path, err)
		}
		if m.Name == "" {
			return nil, fmt.Errorf("manifest %s: 'name' field is required", path)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("no manifest found in %s (tried .ratchet-plugin/plugin.json, .claude-plugin/plugin.json)", pluginDir)
}
