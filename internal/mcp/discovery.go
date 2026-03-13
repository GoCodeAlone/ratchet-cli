// Package mcp provides CLI-based MCP tool discovery and registration.
package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/GoCodeAlone/ratchet/plugin"
	"github.com/GoCodeAlone/ratchet/ratchetplugin"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// CLISpec describes a CLI tool and the MCP tools derived from it.
type CLISpec struct {
	Name  string
	Tools []cliTool
}

// cliTool wraps a shell command as a plugin.Tool.
type cliTool struct {
	name    string
	desc    string
	cmdArgs []string // args passed to exec, with {args} as placeholder for user input
}

func (t *cliTool) Name() string        { return t.name }
func (t *cliTool) Description() string { return t.desc }
func (t *cliTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        t.name,
		Description: t.desc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"args": map[string]any{
					"type":        "string",
					"description": "Additional arguments to pass to the CLI command",
				},
			},
		},
	}
}
func (t *cliTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	extra := ""
	if v, ok := args["args"]; ok {
		extra, _ = v.(string)
	}
	cmdArgs := append(t.cmdArgs, extra) //nolint:gocritic
	out, err := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w\n%s", t.name, err, out)
	}
	return string(out), nil
}

// knownCLIs maps binary name to its derived MCP tool set.
var knownCLIs = []CLISpec{
	{
		Name: "gh",
		Tools: []cliTool{
			{
				name:    "github_issues",
				desc:    "List or search GitHub issues via gh CLI",
				cmdArgs: []string{"gh", "issue", "list"},
			},
			{
				name:    "github_prs",
				desc:    "List or search GitHub pull requests via gh CLI",
				cmdArgs: []string{"gh", "pr", "list"},
			},
			{
				name:    "github_repos",
				desc:    "List GitHub repositories via gh CLI",
				cmdArgs: []string{"gh", "repo", "list"},
			},
		},
	},
	{
		Name: "docker",
		Tools: []cliTool{
			{
				name:    "docker_ps",
				desc:    "List running Docker containers",
				cmdArgs: []string{"docker", "ps"},
			},
			{
				name:    "docker_logs",
				desc:    "Fetch Docker container logs",
				cmdArgs: []string{"docker", "logs"},
			},
			{
				name:    "docker_exec",
				desc:    "Execute a command in a Docker container",
				cmdArgs: []string{"docker", "exec"},
			},
		},
	},
	{
		Name: "kubectl",
		Tools: []cliTool{
			{
				name:    "kubectl_get",
				desc:    "Get Kubernetes resources",
				cmdArgs: []string{"kubectl", "get"},
			},
			{
				name:    "kubectl_logs",
				desc:    "Fetch Kubernetes pod logs",
				cmdArgs: []string{"kubectl", "logs"},
			},
			{
				name:    "kubectl_describe",
				desc:    "Describe Kubernetes resources",
				cmdArgs: []string{"kubectl", "describe"},
			},
		},
	},
}

// DiscoveryResult is the result of a CLI discovery run.
type DiscoveryResult struct {
	// Registered maps CLI name to the tool names registered.
	Registered map[string][]string
}

// Discoverer wraps CLI discovery and caches results.
type Discoverer struct {
	registry  *ratchetplugin.ToolRegistry
	mu        sync.Mutex
	cached    *DiscoveryResult
	lookPath  func(string) (string, error) // injectable for tests
}

// NewDiscoverer creates a Discoverer backed by the given ToolRegistry.
func NewDiscoverer(registry *ratchetplugin.ToolRegistry) *Discoverer {
	return &Discoverer{
		registry: registry,
		lookPath: exec.LookPath,
	}
}

// Discover detects available CLIs and registers their tools.
// Results are cached; subsequent calls return the cached result immediately.
func (d *Discoverer) Discover() *DiscoveryResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cached != nil {
		return d.cached
	}

	result := &DiscoveryResult{
		Registered: make(map[string][]string),
	}

	for _, spec := range knownCLIs {
		if _, err := d.lookPath(spec.Name); err != nil {
			continue // CLI not found
		}
		tools := make([]plugin.Tool, len(spec.Tools))
		names := make([]string, len(spec.Tools))
		for i := range spec.Tools {
			t := spec.Tools[i]
			tools[i] = &t
			names[i] = t.name
		}
		d.registry.RegisterMCP(spec.Name, tools)
		result.Registered[spec.Name] = names
	}

	d.cached = result
	return result
}

// InvalidateCache forces the next Discover() call to re-detect CLIs.
func (d *Discoverer) InvalidateCache() {
	d.mu.Lock()
	d.cached = nil
	d.mu.Unlock()
}

// Enable re-runs discovery (cache cleared first) and returns the result.
func (d *Discoverer) Enable(cliName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, spec := range knownCLIs {
		if spec.Name != cliName {
			continue
		}
		if _, err := d.lookPath(spec.Name); err != nil {
			return fmt.Errorf("CLI %q not found in PATH", cliName)
		}
		tools := make([]plugin.Tool, len(spec.Tools))
		for i := range spec.Tools {
			t := spec.Tools[i]
			tools[i] = &t
		}
		d.registry.RegisterMCP(spec.Name, tools)
		if d.cached != nil {
			d.cached.Registered[cliName] = toolNames(spec.Tools)
		}
		return nil
	}
	return fmt.Errorf("unknown CLI %q", cliName)
}

// Disable removes tools for the given CLI from the registry.
func (d *Discoverer) Disable(cliName string) {
	d.registry.UnregisterMCP(cliName)
	d.mu.Lock()
	if d.cached != nil {
		delete(d.cached.Registered, cliName)
	}
	d.mu.Unlock()
}

func toolNames(tools []cliTool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.name
	}
	return names
}
