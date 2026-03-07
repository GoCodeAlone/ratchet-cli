package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// InstructionSource represents a discovered instruction file.
type InstructionSource struct {
	Path    string
	Source  string // "claude", "copilot", "cursor", "windsurf", "ratchet"
	Scope   string // "global", "project"
	Content string
}

// DiscoverInstructions finds all instruction files for the given working directory.
// enabledSources controls which tool conventions to discover (e.g., ["claude", "copilot"]).
// providerFilter limits model-specific files to the current provider (e.g., "anthropic").
func DiscoverInstructions(workingDir string, enabledSources []string, providerFilter string) []InstructionSource {
	enabled := make(map[string]bool)
	for _, s := range enabledSources {
		enabled[s] = true
	}

	var instructions []InstructionSource

	home, _ := os.UserHomeDir()
	ratchetDir := filepath.Join(home, ".ratchet")

	// 1. Global user instructions
	instructions = appendIfExists(instructions, filepath.Join(ratchetDir, "instructions.md"), "ratchet", "global")

	// 2. Project-level single files
	projectFiles := []struct {
		file   string
		source string
	}{
		{"CLAUDE.md", "claude"},
		{"AGENTS.md", "copilot"},
		{".github/copilot-instructions.md", "copilot"},
		{".cursorrules", "cursor"},
		{".windsurfrules", "windsurf"},
		{"RATCHET.md", "ratchet"},
	}
	for _, pf := range projectFiles {
		if pf.source != "ratchet" && !enabled[pf.source] {
			continue
		}
		instructions = appendIfExists(instructions, filepath.Join(workingDir, pf.file), pf.source, "project")
	}

	// 3. Directory-based instructions (load all .md files in dirs)
	instructionDirs := []struct {
		dir    string
		source string
	}{
		{".claude", "claude"},
		{".github/instructions", "copilot"},
		{".cursor/rules", "cursor"},
		{".ratchet/instructions", "ratchet"},
	}
	for _, id := range instructionDirs {
		if id.source != "ratchet" && !enabled[id.source] {
			continue
		}
		dirPath := filepath.Join(workingDir, id.dir)
		instructions = appendDirFiles(instructions, dirPath, id.source, "project")
	}

	// 4. Model-specific overrides (global + project)
	if providerFilter != "" {
		globalModel := filepath.Join(ratchetDir, "instructions."+providerFilter+".md")
		instructions = appendIfExists(instructions, globalModel, "ratchet", "global")

		projectModel := filepath.Join(workingDir, ".ratchet", "instructions."+providerFilter+".md")
		instructions = appendIfExists(instructions, projectModel, "ratchet", "project")
	}

	return instructions
}

// BuildSystemPrompt concatenates all discovered instructions into a system prompt.
func BuildSystemPrompt(instructions []InstructionSource) string {
	if len(instructions) == 0 {
		return ""
	}
	var parts []string
	for _, inst := range instructions {
		parts = append(parts, "# Instructions from "+inst.Path+"\n\n"+inst.Content)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func appendIfExists(instructions []InstructionSource, path, source, scope string) []InstructionSource {
	data, err := os.ReadFile(path)
	if err != nil {
		return instructions
	}
	return append(instructions, InstructionSource{
		Path:    path,
		Source:  source,
		Scope:   scope,
		Content: strings.TrimSpace(string(data)),
	})
}

func appendDirFiles(instructions []InstructionSource, dirPath, source, scope string) []InstructionSource {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return instructions
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".txt") {
			continue
		}
		path := filepath.Join(dirPath, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		instructions = append(instructions, InstructionSource{
			Path:    path,
			Source:  source,
			Scope:   scope,
			Content: strings.TrimSpace(string(data)),
		})
	}
	return instructions
}
