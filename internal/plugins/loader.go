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

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
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
	Skills      []skills.Skill
	Agents      []agent.AgentDefinition
	Commands    []Command
	Hooks       *hooks.HookConfig
	Tools       []plugin.Tool
	MCPConfigs  []MCPConfig
	ACPProfiles []acpclient.Profile
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

// DefaultDir returns the default plugin installation directory.
func DefaultDir() string {
	return pluginsDir()
}

// LoadAll scans pluginDir for plugin directories, parses their manifests, and
// returns all discovered capabilities aggregated in a LoadResult.
// Daemon tools are started using ctx; cancel ctx to terminate them.
func (l *Loader) LoadAll(ctx context.Context) (*LoadResult, error) {
	disabled := disabledPluginSet()
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
		if disabled[entry.Name()] {
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

// LoadSkills scans installed plugins and returns only skill capabilities. It is
// intentionally passive: tool daemons and MCP discovery are not started.
func (l *Loader) LoadSkills() ([]skills.Skill, error) {
	disabled := disabledPluginSet()
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}
	var result []skills.Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if disabled[entry.Name()] {
			continue
		}
		pluginDir := filepath.Join(l.pluginDir, entry.Name())
		m, err := LoadManifest(pluginDir)
		if err != nil {
			continue
		}
		if m.Capabilities.Skills == "" {
			continue
		}
		skillsPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Skills)
		if err != nil {
			return nil, fmt.Errorf("load plugin %s skills: %w", m.Name, err)
		}
		loaded, err := loadSkills(skillsPath)
		if err != nil {
			return nil, fmt.Errorf("load plugin %s skills: %w", m.Name, err)
		}
		for i := range loaded {
			loaded[i].Source = "plugin"
			loaded[i].PluginName = m.Name
			loaded[i].PluginVersion = m.Version
		}
		result = append(result, loaded...)
	}
	return result, nil
}

func disabledPluginSet() map[string]bool {
	reg, err := Load()
	if err != nil {
		log.Printf("plugins: disabled registry state unavailable: %v", err)
		return nil
	}
	disabled := make(map[string]bool)
	for name, entry := range reg.Plugins {
		if !entry.Enabled {
			disabled[name] = true
		}
	}
	return disabled
}

// loadPlugin loads a single plugin's capabilities into result.
func (l *Loader) loadPlugin(ctx context.Context, pluginDir string, m *Manifest, result *LoadResult) error {
	if m.Capabilities.Skills != "" {
		skillsPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Skills)
		if err != nil {
			return fmt.Errorf("skills: %w", err)
		}
		s, err := loadSkills(skillsPath)
		if err != nil {
			return fmt.Errorf("skills: %w", err)
		}
		for i := range s {
			s[i].Source = "plugin"
			s[i].PluginName = m.Name
			s[i].PluginVersion = m.Version
		}
		result.Skills = append(result.Skills, s...)
	}

	if m.Capabilities.Agents != "" {
		agentsPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Agents)
		if err != nil {
			return fmt.Errorf("agents: %w", err)
		}
		a, err := loadAgents(agentsPath)
		if err != nil {
			return fmt.Errorf("agents: %w", err)
		}
		result.Agents = append(result.Agents, a...)
	}

	if m.Capabilities.Commands != "" {
		commandsPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Commands)
		if err != nil {
			return fmt.Errorf("commands: %w", err)
		}
		c, err := loadCommands(commandsPath)
		if err != nil {
			return fmt.Errorf("commands: %w", err)
		}
		result.Commands = append(result.Commands, c...)
	}

	if m.Capabilities.Hooks != "" {
		hooksPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Hooks)
		if err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
		hc, err := loadHooks(hooksPath)
		if err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
		hc.AnnotateSource(hooks.SourceMetadata{
			Kind:          hooks.SourcePlugin,
			ID:            fmt.Sprintf("plugin:%s@%s:%s", m.Name, m.Version, filepath.ToSlash(filepath.Clean(m.Capabilities.Hooks))),
			Path:          hooksPath,
			PluginName:    m.Name,
			PluginVersion: m.Version,
		})
		// Merge plugin hooks into result hooks.
		for event, hookList := range hc.Hooks {
			result.Hooks.Hooks[event] = append(result.Hooks.Hooks[event], hookList...)
		}
	}

	if m.Capabilities.Tools != "" {
		toolsPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.Tools)
		if err != nil {
			return fmt.Errorf("tools: %w", err)
		}
		tools, daemons, err := loadTools(ctx, toolsPath)
		if err != nil {
			return fmt.Errorf("tools: %w", err)
		}
		result.Tools = append(result.Tools, tools...)
		result.Daemons = append(result.Daemons, daemons...)
	}

	if m.Capabilities.MCP != "" {
		mcpPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.MCP)
		if err != nil {
			return fmt.Errorf("mcp: %w", err)
		}
		mc, err := loadMCPConfig(m.Name, pluginDir, mcpPath)
		if err != nil {
			return fmt.Errorf("mcp: %w", err)
		}
		result.MCPConfigs = append(result.MCPConfigs, mc)
	}

	if m.Capabilities.ACPProfiles != "" {
		profilesPath, err := resolveCapabilityPath(pluginDir, m.Capabilities.ACPProfiles)
		if err != nil {
			return fmt.Errorf("acpProfiles: %w", err)
		}
		profiles, err := loadACPProfiles(profilesPath, m, m.Capabilities.ACPProfiles)
		if err != nil {
			return fmt.Errorf("acpProfiles: %w", err)
		}
		result.ACPProfiles = append(result.ACPProfiles, profiles...)
	}

	return nil
}

func resolveCapabilityPath(pluginDir, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty capability path")
	}
	firstSegment := strings.Split(filepath.ToSlash(rel), "/")[0]
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.Contains(firstSegment, ":") {
		return "", fmt.Errorf("capability path %q must be relative", rel)
	}
	root, err := filepath.Abs(pluginDir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanPath != cleanRoot {
		prefix := cleanRoot + string(os.PathSeparator)
		if !strings.HasPrefix(cleanPath, prefix) {
			return "", fmt.Errorf("capability path %q escapes plugin directory", rel)
		}
	}
	return cleanPath, nil
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

type acpProfilesFile struct {
	Profiles []acpclient.Profile `json:"profiles" yaml:"profiles"`
}

func loadACPProfiles(path string, m *Manifest, rel string) ([]acpclient.Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file acpProfilesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	for i := range file.Profiles {
		p := &file.Profiles[i]
		if p.Spec.Name == "" {
			p.Spec.Name = p.Name
		}
		p.SourceKind = "plugin"
		p.SourceID = fmt.Sprintf("plugin:%s@%s:%s/%s", m.Name, m.Version, filepath.ToSlash(filepath.Clean(rel)), p.Name)
		p.PluginName = m.Name
		p.PluginVersion = m.Version
		p.Hash = p.DescriptorHash()
	}
	return file.Profiles, nil
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
