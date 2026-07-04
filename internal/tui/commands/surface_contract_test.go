package commands

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type surfaceSpec struct {
	Commands  []surfaceCommand  `json:"commands"`
	Shortcuts []surfaceShortcut `json:"shortcuts"`
}

type surfaceCommand struct {
	Command  string `json:"command"`
	Evidence string `json:"evidence"`
}

type surfaceShortcut struct {
	Keys     string `json:"keys"`
	Evidence string `json:"evidence"`
}

func TestCommandSurfaceSpecClassifiesParserHelpAndPTYRows(t *testing.T) {
	spec := loadSurfaceSpec(t)
	for _, row := range spec.Commands {
		switch row.Evidence {
		case "pty-proven", "focused-proven", "deferred-runtime":
		default:
			t.Fatalf("%s has invalid evidence %q", row.Command, row.Evidence)
		}
	}

	for _, cmd := range parserCommandCases(t) {
		if !specCoversTopLevel(spec, cmd) {
			t.Fatalf("parser command %s missing from command_surface_spec.json", cmd)
		}
	}
	for _, cmd := range helpCommandRows() {
		if !specCoversTopLevel(spec, cmd) {
			t.Fatalf("help command %s missing from command_surface_spec.json", cmd)
		}
	}
	for _, cmd := range []string{
		"/mode conservative",
		"/mode permissive",
		"/mode locked",
		"/mode sandbox",
		"/mode custom",
		"/trust allow \"smoke:allow\" --scope smoke",
		"/trust deny \"smoke:deny\" --scope smoke",
		"/trust persist allow \"smoke:persist-allow\" --scope smoke",
		"/trust persist deny \"smoke:persist-deny\" --scope smoke",
		"/trust revoke \"smoke:persist-allow\" --scope smoke",
	} {
		if !specHasEvidence(spec, cmd, "pty-proven") {
			t.Fatalf("%s must remain pty-proven in command_surface_spec.json", cmd)
		}
	}
}

func TestCommandSurfaceSpecCoversFocusedSlashCommands(t *testing.T) {
	spec := loadSurfaceSpec(t)
	for _, cmd := range []string{
		"/model",
		"/clear",
		"/cost",
		"/agents",
		"/sessions",
		"/provider add",
		"/provider remove <alias>",
		"/provider default <alias>",
		"/provider test <alias>",
		"/loop <interval> <command>",
		"/cron <expr> <command>",
		"/cron list",
		"/cron pause <id>",
		"/cron resume <id>",
		"/cron stop <id>",
		"/fleet <plan_id>",
		"/team status <id>",
		"/team start <task>",
		"/plan",
		"/approve <plan_id>",
		"/reject <plan_id>",
		"/jobs",
		"/compact",
		"/review",
		"/login [alias]",
		"/mcp list",
		"/mcp enable <name>",
		"/mcp disable <name>",
		"/mode <mode>",
		"/trust allow \"pattern\" [--scope scope]",
		"/trust deny \"pattern\" [--scope scope]",
		"/trust persist allow \"pattern\" [--scope scope]",
		"/trust persist deny \"pattern\" [--scope scope]",
		"/trust revoke \"pattern\" [--scope scope]",
	} {
		if !specHasEvidence(spec, cmd, "focused-proven") {
			t.Fatalf("%s must be focused-proven in command_surface_spec.json", cmd)
		}
	}
}

func loadSurfaceSpec(t *testing.T) surfaceSpec {
	t.Helper()
	data, err := os.ReadFile("testdata/command_surface_spec.json")
	if err != nil {
		t.Fatalf("read command surface spec: %v", err)
	}
	var spec surfaceSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse command surface spec: %v", err)
	}
	return spec
}

func parserCommandCases(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("commands.go")
	if err != nil {
		t.Fatalf("read commands.go: %v", err)
	}
	matches := regexp.MustCompile(`case "(/[^"]+)"`).FindAllStringSubmatch(string(data), -1)
	var out []string
	for _, match := range matches {
		if !slices.Contains(out, match[1]) {
			out = append(out, match[1])
		}
	}
	return out
}

func helpCommandRows() []string {
	var out []string
	for _, line := range helpCmd().Lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") {
			out = append(out, strings.Fields(line)[0])
		}
	}
	return out
}

func specCoversTopLevel(spec surfaceSpec, cmd string) bool {
	for _, row := range spec.Commands {
		if row.Command == cmd || strings.HasPrefix(row.Command, cmd+" ") {
			return true
		}
	}
	return false
}

func specHasEvidence(spec surfaceSpec, command, evidence string) bool {
	for _, row := range spec.Commands {
		if row.Command == command && row.Evidence == evidence {
			return true
		}
	}
	return false
}
