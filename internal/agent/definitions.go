package agent

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtins/*.yaml
var builtinFS embed.FS

// LoadBuiltins returns the built-in agent definitions embedded in the binary.
func LoadBuiltins() ([]AgentDefinition, error) {
	entries, err := builtinFS.ReadDir("builtins")
	if err != nil {
		return nil, fmt.Errorf("read builtins: %w", err)
	}
	var defs []AgentDefinition
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := builtinFS.ReadFile("builtins/" + e.Name())
		if err != nil {
			continue
		}
		var def AgentDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			continue
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(e.Name(), ext)
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// AgentDefinition defines a reusable AI agent configuration.
type AgentDefinition struct {
	Name          string   `yaml:"name"`
	Role          string   `yaml:"role"`
	Model         string   `yaml:"model"`
	Provider      string   `yaml:"provider"`
	SystemPrompt  string   `yaml:"system_prompt"`
	Tools         []string `yaml:"tools"`
	MaxIterations int      `yaml:"max_iterations"`
}

// EffectiveProvider returns the agent's provider, falling back to defaultProvider if unset.
func (d AgentDefinition) EffectiveProvider(defaultProvider string) string {
	if d.Provider != "" {
		return d.Provider
	}
	return defaultProvider
}

// EffectiveModel returns the agent's model, falling back to defaultModel if unset.
func (d AgentDefinition) EffectiveModel(defaultModel string) string {
	if d.Model != "" {
		return d.Model
	}
	return defaultModel
}

// LoadDefinitions discovers agent definitions from standard locations.
// Searches: ~/.ratchet/agents/*.yaml, .ratchet/agents/*.yaml, .claude/agents/*.md
func LoadDefinitions(workingDir string) ([]AgentDefinition, error) {
	var defs []AgentDefinition

	home, _ := os.UserHomeDir()
	searchDirs := []struct {
		dir    string
		format string
	}{
		{filepath.Join(home, ".ratchet", "agents"), "yaml"},
		{filepath.Join(workingDir, ".ratchet", "agents"), "yaml"},
		{filepath.Join(workingDir, ".claude", "agents"), "md"},
	}

	seen := make(map[string]bool)
	for _, sd := range searchDirs {
		entries, err := os.ReadDir(sd.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read dir %s: %w", sd.dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if (sd.format == "yaml" && (ext != ".yaml" && ext != ".yml")) ||
				(sd.format == "md" && ext != ".md") {
				continue
			}
			path := filepath.Join(sd.dir, e.Name())
			var def AgentDefinition
			var err error
			if sd.format == "md" {
				def, err = parseMarkdownAgent(path)
			} else {
				def, err = parseYAMLAgent(path)
			}
			if err != nil {
				continue // skip malformed files
			}
			if def.Name == "" {
				def.Name = strings.TrimSuffix(e.Name(), ext)
			}
			if !seen[def.Name] {
				seen[def.Name] = true
				defs = append(defs, def)
			}
		}
	}
	return defs, nil
}

// CreateDefinition writes a new agent definition YAML to ~/.ratchet/agents/<name>.yaml.
func CreateDefinition(def AgentDefinition) error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ratchet", "agents")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, def.Name+".yaml")
	data, err := yaml.Marshal(def)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func parseYAMLAgent(path string) (AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentDefinition{}, err
	}
	var def AgentDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return AgentDefinition{}, err
	}
	return def, nil
}

// parseMarkdownAgent parses a Claude Code agent markdown file with YAML front matter.
func parseMarkdownAgent(path string) (AgentDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return AgentDefinition{}, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var frontMatter strings.Builder
	inFrontMatter := false
	var bodyLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if !inFrontMatter && line == "---" {
			inFrontMatter = true
			continue
		}
		if inFrontMatter {
			if line == "---" {
				inFrontMatter = false
				continue
			}
			frontMatter.WriteString(line + "\n")
		} else {
			bodyLines = append(bodyLines, line)
		}
	}

	var def AgentDefinition
	if frontMatter.Len() > 0 {
		_ = yaml.Unmarshal([]byte(frontMatter.String()), &def)
	}
	if def.SystemPrompt == "" {
		def.SystemPrompt = strings.Join(bodyLines, "\n")
	}
	return def, nil
}
