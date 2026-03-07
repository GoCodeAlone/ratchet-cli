package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a discovered and loaded skill.
type Skill struct {
	Name    string
	Path    string
	Content string
}

// Discover finds skills from ~/.ratchet/skills/ and .ratchet/skills/.
// Project skills take precedence over global skills.
func Discover(workingDir string) []Skill {
	home, _ := os.UserHomeDir()
	searchDirs := []string{
		filepath.Join(home, ".ratchet", "skills"),
		filepath.Join(workingDir, ".ratchet", "skills"),
	}

	seen := make(map[string]bool)
	var skills []Skill

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if ext != ".md" && ext != ".txt" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ext)
			path := filepath.Join(dir, e.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			// Later paths (project-level) override earlier paths (global)
			seen[name] = true
			skills = append(skills, Skill{
				Name:    name,
				Path:    path,
				Content: string(content),
			})
		}
	}
	_ = seen
	return dedup(skills)
}

// dedup returns skills with project-level overriding global.
func dedup(skills []Skill) []Skill {
	latest := make(map[string]Skill)
	var order []string
	for _, s := range skills {
		if _, exists := latest[s.Name]; !exists {
			order = append(order, s.Name)
		}
		latest[s.Name] = s
	}
	result := make([]Skill, 0, len(order))
	for _, name := range order {
		result = append(result, latest[name])
	}
	return result
}

// Inject augments a system prompt with relevant skill content.
func Inject(systemPrompt string, skills []Skill) string {
	if len(skills) == 0 {
		return systemPrompt
	}
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n## Available Skills\n\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", s.Name, s.Content))
	}
	return sb.String()
}
