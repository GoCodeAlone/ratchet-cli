package main

import (
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
	"github.com/GoCodeAlone/ratchet-cli/internal/skills"
)

func handleSkill(args []string) {
	wd, _ := os.Getwd()
	if len(args) == 0 {
		fmt.Println("Usage: ratchet skill <list|show>")
		return
	}
	switch args[0] {
	case "list":
		discovered := discoverCLISkills(wd)
		if len(discovered) == 0 {
			fmt.Println("No skills found.")
			return
		}
		for _, s := range discovered {
			source := s.Source
			if source == "" {
				source = "-"
			}
			fmt.Printf("%-28s %-8s %s\n", s.Name, source, s.Path)
		}
	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet skill show <name>")
			return
		}
		discovered := discoverCLISkills(wd)
		for _, s := range discovered {
			if s.Name == args[1] {
				fmt.Println(s.Content)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "skill not found: %s\n", args[1])
		os.Exit(1)
	default:
		fmt.Printf("unknown skill command: %s\n", args[0])
	}
}

func discoverCLISkills(wd string) []skills.Skill {
	pluginSkills := loadPluginSkills()
	return skills.Merge(pluginSkills, skills.NamespacedAliases(pluginSkills), skills.Discover(wd))
}

func loadPluginSkills() []skills.Skill {
	result, err := plugins.NewLoader(plugins.DefaultDir()).LoadSkills()
	if err != nil {
		return nil
	}
	return result
}
