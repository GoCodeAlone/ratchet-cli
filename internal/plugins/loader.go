package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/skills"
	"github.com/GoCodeAlone/workflow-plugin-agent/plugin"
)

// Command represents a slash command contributed by a plugin.
type Command struct {
	Name    string // slash command name (derived from filename without extension)
	Content string // full markdown content
	Path    string
}

// MCPServerSpec describes a single MCP server entry from .mcp.json.
type MCPServerSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPConfig holds all MCP server declarations from a plugin's .mcp.json.
type MCPConfig struct {
	PluginName string
	PluginDir  string
	Servers    map[string]MCPServerSpec `json:"mcpServers"`
}

// LoadResult aggregates all capabilities discovered across loaded plugins.
type LoadResult struct {
	Skills     []skills.Skill
	Agents     []agent.AgentDefinition
	Commands   []Command
	Hooks      *hooks.HookConfig
	Tools      []plugin.Tool
	MCPConfigs []MCPConfig
	// Daemons holds all started daemon processes so callers can stop them.
	// Call StopDaemons() to cleanly shut them all down.
	Daemons []*DaemonTool
}

// StopDaemons stops all daemon tool processes tracked by this LoadResult.
func (r *LoadResult) StopDaemons() {
	stopDaemons(r.Daemons)
}

// Loader discovers and loads plugins from a directory.
type Loader struct {
	pluginDir string
}

// NewLoader creates a Loader targeting pluginDir.
func NewLoader(pluginDir string) *Loader {
	return &Loader{pluginDir: pluginDir}
}

// pluginsDir returns the default plugin installation directory.
func pluginsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "plugins")
}

// LoadAll scans pluginDir for plugin directories, parses their manifests, and
// returns all discovered capabilities aggregated in a LoadResult.
// Daemon tools are started using ctx; cancel ctx to terminate them.
func (l *Loader) LoadAll(ctx context.Context) (*LoadResult, error) {
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &LoadResult{Hooks: &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}}, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	result := &LoadResult{
		Hooks: &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)},
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(l.pluginDir, entry.Name())
		m, err := LoadManifest(pluginDir)
		if err != nil {
			// Not a plugin directory — skip silently.
			continue
		}
		if err := l.loadPlugin(ctx, pluginDir, m, result); err != nil {
			return nil, fmt.Errorf("load plugin %s: %w", m.Name, err)
		}
	}
	return result, nil
}

// loadPlugin loads a single plugin's capabilities into result.
func (l *Loader) loadPlugin(ctx context.Context, pluginDir string, m *Manifest, result *LoadResult) error {
	if m.Capabilities.Skills != "" {
		s, err := loadSkills(filepath.Join(pluginDir, m.Capabilities.Skills))
		if err != nil {
			return fmt.Errorf("skills: %w", err)
		}
		result.Skills = append(result.Skills, s...)
	}

	if m.Capabilities.Agents != "" {
		a, err := loadAgents(filepath.Join(pluginDir, m.Capabilities.Agents))
		if err != nil {
			return fmt.Errorf("agents: %w", err)
		}
		result.Agents = append(result.Agents, a...)
	}

	if m.Capabilities.Commands != "" {
		c, err := loadCommands(filepath.Join(pluginDir, m.Capabilities.Commands))
		if err != nil {
			return fmt.Errorf("commands: %w", err)
		}
		result.Commands = append(result.Commands, c...)
	}

	if m.Capabilities.Hooks != "" {
		hc, err := loadHooks(filepath.Join(pluginDir, m.Capabilities.Hooks))
		if err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
		// Merge plugin hooks into result hooks.
		for event, hookList := range hc.Hooks {
			result.Hooks.Hooks[event] = append(result.Hooks.Hooks[event], hookList...)
		}
	}

	if m.Capabilities.Tools != "" {
		tools, daemons, err := loadTools(ctx, filepath.Join(pluginDir, m.Capabilities.Tools))
		if err != nil {
			return fmt.Errorf("tools: %w", err)
		}
		result.Tools = append(result.Tools, tools...)
		result.Daemons = append(result.Daemons, daemons...)
	}

	if m.Capabilities.MCP != "" {
		mc, err := loadMCPConfig(m.Name, pluginDir, filepath.Join(pluginDir, m.Capabilities.MCP))
		if err != nil {
			return fmt.Errorf("mcp: %w", err)
		}
		result.MCPConfigs = append(result.MCPConfigs, mc)
	}

	return nil
}

// loadSkills reads SKILL.md files from skill subdirectories.
func loadSkills(skillsDir string) ([]skills.Skill, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []skills.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue // skip skill dirs without SKILL.md
		}
		result = append(result, skills.Skill{
			Name:    e.Name(),
			Path:    skillFile,
			Content: string(content),
		})
	}
	return result, nil
}

// loadAgents reads *.yaml agent definition files from agentsDir.
func loadAgents(agentsDir string) ([]agent.AgentDefinition, error) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []agent.AgentDefinition
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentsDir, e.Name()))
		if err != nil {
			continue
		}
		var def agent.AgentDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			continue
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(e.Name(), ext)
		}
		result = append(result, def)
	}
	return result, nil
}

// loadCommands reads *.md command files from commandsDir.
func loadCommands(commandsDir string) ([]Command, error) {
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []Command
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		path := filepath.Join(commandsDir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		result = append(result, Command{
			Name:    name,
			Content: string(content),
			Path:    path,
		})
	}
	return result, nil
}

// loadHooks parses a hooks JSON/YAML file into a HookConfig.
func loadHooks(hooksFile string) (*hooks.HookConfig, error) {
	data, err := os.ReadFile(hooksFile)
	if err != nil {
		return nil, err
	}
	var hc hooks.HookConfig
	// yaml.Unmarshal handles JSON too (JSON is a subset of YAML).
	if err := yaml.Unmarshal(data, &hc); err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}
	if hc.Hooks == nil {
		hc.Hooks = make(map[hooks.Event][]hooks.Hook)
	}
	return &hc, nil
}

// loadTools scans toolsDir for tool subdirectories and creates ExecTool or
// DaemonTool instances based on the protocol declared in tool.json.
// It returns all tool references and the started daemon processes separately
// so callers can stop the daemons when done.
func loadTools(ctx context.Context, toolsDir string) ([]plugin.Tool, []*DaemonTool, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var result []plugin.Tool
	var startedDaemons []*DaemonTool
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		toolDir := filepath.Join(toolsDir, e.Name())
		defData, err := os.ReadFile(filepath.Join(toolDir, "tool.json"))
		if err != nil {
			continue // not a tool directory
		}
		var def ToolDef
		if err := json.Unmarshal(defData, &def); err != nil {
			log.Printf("plugins: skipping %s: parse tool.json: %v", e.Name(), err)
			continue
		}
		if def.Name == "" {
			log.Printf("plugins: skipping %s: tool.json missing 'name' field", e.Name())
			continue
		}

		switch def.Protocol {
		case "daemon":
			binPath, err := findBinary(toolDir, def.Name)
			if err != nil {
				stopDaemons(startedDaemons)
				return nil, nil, fmt.Errorf("tool %s: %w", def.Name, err)
			}
			daemon, err := StartDaemon(ctx, binPath)
			if err != nil {
				stopDaemons(startedDaemons)
				return nil, nil, fmt.Errorf("start daemon %s: %w", def.Name, err)
			}
			startedDaemons = append(startedDaemons, daemon)
			for _, d := range daemon.Defs() {
				result = append(result, NewDaemonToolRef(daemon, d))
			}
		default: // "exec" or unspecified
			t, err := LoadExecTool(toolDir)
			if err != nil {
				stopDaemons(startedDaemons)
				return nil, nil, fmt.Errorf("tool %s: %w", def.Name, err)
			}
			result = append(result, t)
		}
	}
	return result, startedDaemons, nil
}

// stopDaemons stops all daemon tools in the slice (used for cleanup on error).
func stopDaemons(daemons []*DaemonTool) {
	for _, d := range daemons {
		_ = d.Stop()
	}
}

// loadMCPConfig reads a .mcp.json file and returns an MCPConfig.
func loadMCPConfig(pluginName, pluginDir, mcpFile string) (MCPConfig, error) {
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		return MCPConfig{}, err
	}
	var mc MCPConfig
	if err := json.Unmarshal(data, &mc); err != nil {
		return MCPConfig{}, fmt.Errorf("parse .mcp.json: %w", err)
	}
	mc.PluginName = pluginName
	mc.PluginDir = pluginDir
	return mc, nil
}
