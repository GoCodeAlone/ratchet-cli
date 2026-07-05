package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Skill represents a discovered and loaded skill.
type Skill struct {
	Name          string
	Path          string
	Content       string
	Source        string
	PluginName    string
	PluginVersion string
}

// Discover finds skills from ~/.ratchet/skills/ and .ratchet/skills/.
// Project skills take precedence over global skills.
func Discover(workingDir string) []Skill {
	home, _ := os.UserHomeDir()
	searchDirs := []string{
		filepath.Join(home, ".ratchet", "skills"),
		filepath.Join(workingDir, ".ratchet", "skills"),
	}

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
			skills = append(skills, Skill{
				Name:    name,
				Path:    path,
				Content: string(content),
				Source:  sourceForSkillPath(home, workingDir, path),
			})
		}
	}
	return dedup(skills)
}

func sourceForSkillPath(home, workingDir, path string) string {
	projectDir := filepath.Join(workingDir, ".ratchet", "skills")
	if rel, err := filepath.Rel(projectDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "project"
	}
	userDir := filepath.Join(home, ".ratchet", "skills")
	if rel, err := filepath.Rel(userDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "user"
	}
	return ""
}

// Merge combines skill lists while preserving first-seen order and allowing
// later lists to override same-name content.
func Merge(skillLists ...[]Skill) []Skill {
	var all []Skill
	for _, list := range skillLists {
		all = append(all, list...)
	}
	return dedup(all)
}

// NamespacedAliases returns plugin-name aliases for plugin skills. Legacy
// unqualified skill names are left intact by the plugin loader for backwards
// compatibility; aliases make explicit invocations like $autodev:using-autodev
// resolvable without changing older tests or installed plugin layouts.
func NamespacedAliases(pluginSkills []Skill) []Skill {
	aliases := make([]Skill, 0, len(pluginSkills))
	for _, s := range pluginSkills {
		if s.PluginName == "" || strings.Contains(s.Name, ":") {
			continue
		}
		alias := s
		alias.Name = s.PluginName + ":" + s.Name
		aliases = append(aliases, alias)
	}
	return aliases
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
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", s.Name, s.Content)
	}
	return sb.String()
}

// InjectForPrompt adds a compact skill index plus full skill contents for
// skills explicitly referenced by the user prompt.
func InjectForPrompt(systemPrompt string, available []Skill, userPrompt string) string {
	if len(available) == 0 {
		return systemPrompt
	}
	selected := SelectForPrompt(available, userPrompt)
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n## Available Skills Index\n\n")
	for _, s := range available {
		fmt.Fprintf(&sb, "- `%s`", s.Name)
		if s.Source != "" {
			fmt.Fprintf(&sb, " source=%s", s.Source)
		}
		if desc := Description(s); desc != "" {
			fmt.Fprintf(&sb, " - %s", desc)
		}
		sb.WriteString("\n")
	}
	if len(selected) == 0 {
		return sb.String()
	}
	sb.WriteString("\n## Active Skills\n\n")
	for _, s := range selected {
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", s.Name, s.Content)
	}
	return sb.String()
}

// SelectForPrompt returns skills explicitly referenced by name in the prompt.
func SelectForPrompt(available []Skill, userPrompt string) []Skill {
	if strings.TrimSpace(userPrompt) == "" {
		return nil
	}
	mentioned := make(map[string]bool)
	for _, token := range promptTokens(userPrompt) {
		mentioned[token] = true
	}
	var selected []Skill
	seen := make(map[string]bool)
	for _, s := range available {
		if mentioned[s.Name] && !seen[s.Name] {
			selected = append(selected, s)
			seen[s.Name] = true
		}
	}
	return selected
}

var skillTokenRE = regexp.MustCompile(`[$/]?([A-Za-z0-9_.-]+(?::[A-Za-z0-9_.-]+)?)`)

func promptTokens(prompt string) []string {
	matches := skillTokenRE.FindAllStringSubmatch(prompt, -1)
	tokens := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			tokens = append(tokens, match[1])
		}
	}
	return tokens
}

// Description extracts a compact description from common skill frontmatter.
func Description(s Skill) string {
	lines := strings.Split(s.Content, "\n")
	inFrontmatter := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
				continue
			}
			if value, ok := strings.CutPrefix(trimmed, "description:"); ok {
				return strings.Trim(strings.TrimSpace(value), `"'`)
			}
			continue
		}
		break
	}
	return ""
}
