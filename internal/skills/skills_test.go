package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	got := Discover(tmp)
	if len(got) != 0 {
		t.Errorf("expected 0 skills, got %d", len(got))
	}
}

func TestDiscover(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	workDir := filepath.Join(tmp, "work")
	os.MkdirAll(homeDir, 0700)
	os.MkdirAll(workDir, 0700)
	t.Setenv("HOME", homeDir)

	// Global skill
	globalDir := filepath.Join(homeDir, ".ratchet", "skills")
	os.MkdirAll(globalDir, 0700)
	os.WriteFile(filepath.Join(globalDir, "commit.md"), []byte("# Commit skill"), 0600)

	// Project skill (overrides global)
	projDir := filepath.Join(workDir, ".ratchet", "skills")
	os.MkdirAll(projDir, 0700)
	os.WriteFile(filepath.Join(projDir, "commit.md"), []byte("# Project commit skill"), 0600)
	os.WriteFile(filepath.Join(projDir, "review.md"), []byte("# Review skill"), 0600)

	skills := Discover(workDir)
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}

	// Verify project-level overrides global
	for _, s := range skills {
		if s.Name == "commit" && s.Content != "# Project commit skill" {
			t.Errorf("expected project commit skill, got: %s", s.Content)
		}
	}
}

func TestInject(t *testing.T) {
	skills := []Skill{
		{Name: "commit", Content: "Always write good commit messages."},
	}
	result := Inject("You are an AI assistant.", skills)
	if result == "You are an AI assistant." {
		t.Error("expected skills to be injected")
	}
	if len(result) < 50 {
		t.Error("expected non-trivial injected prompt")
	}
}

func TestInjectEmpty(t *testing.T) {
	result := Inject("base prompt", nil)
	if result != "base prompt" {
		t.Errorf("expected unchanged prompt with no skills, got: %s", result)
	}
}

func TestNamespacedAliasesAndExplicitInjection(t *testing.T) {
	pluginSkill := Skill{
		Name:       "using-autodev",
		Source:     "plugin",
		PluginName: "autodev",
		Content:    "---\ndescription: Autodev bootstrap\n---\nUse autodev before responding.",
	}
	available := Merge([]Skill{pluginSkill}, NamespacedAliases([]Skill{pluginSkill}))
	result := InjectForPrompt("base", available, "please proceed $autodev:using-autodev")

	if !strings.Contains(result, "`autodev:using-autodev`") {
		t.Fatalf("expected namespaced skill in index, got:\n%s", result)
	}
	if !strings.Contains(result, "### autodev:using-autodev") {
		t.Fatalf("expected explicit namespaced skill content, got:\n%s", result)
	}
	if !strings.Contains(result, "Autodev bootstrap") {
		t.Fatalf("expected description from frontmatter, got:\n%s", result)
	}
}

func TestInjectForPromptDoesNotInjectUnmentionedSkillContent(t *testing.T) {
	available := []Skill{{Name: "large-skill", Content: "secret large body"}}
	result := InjectForPrompt("base", available, "ordinary prompt")
	if strings.Contains(result, "### large-skill") || strings.Contains(result, "secret large body") {
		t.Fatalf("unexpected full skill body injection:\n%s", result)
	}
	if !strings.Contains(result, "`large-skill`") {
		t.Fatalf("expected skill index entry, got:\n%s", result)
	}
}
