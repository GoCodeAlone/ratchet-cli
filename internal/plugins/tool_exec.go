package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// ToolDef is the on-disk format for a tool's tool.json definition file.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Protocol    string         `json:"protocol"` // "exec" or "daemon"
}

// ExecTool implements plugin.Tool by spawning a new process for each call.
// The binary receives JSON-encoded arguments on stdin and must write a JSON
// result to stdout.
type ExecTool struct {
	def     ToolDef
	binPath string
}

// LoadExecTool reads tool.json from toolDir and locates the binary.
// The binary must be an executable file inside toolDir named after the tool.
func LoadExecTool(toolDir string) (*ExecTool, error) {
	defPath := filepath.Join(toolDir, "tool.json")
	data, err := os.ReadFile(defPath)
	if err != nil {
		return nil, fmt.Errorf("read tool.json: %w", err)
	}
	var def ToolDef
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse tool.json: %w", err)
	}
	if def.Name == "" {
		return nil, fmt.Errorf("tool.json: name is required")
	}

	// Find binary: prefer <toolDir>/<name>, then any executable in toolDir.
	binPath, err := findBinary(toolDir, def.Name)
	if err != nil {
		return nil, err
	}
	return &ExecTool{def: def, binPath: binPath}, nil
}

// findBinary looks for an executable in dir, preferring one named after name.
func findBinary(dir, name string) (string, error) {
	preferred := filepath.Join(dir, name)
	if isExecutable(preferred) {
		return preferred, nil
	}
	// Fall back: scan dir for any executable file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("scan tool dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "tool.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0111 != 0 {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no executable binary found in %s", dir)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

// Name implements plugin.Tool.
func (t *ExecTool) Name() string { return t.def.Name }

// Description implements plugin.Tool.
func (t *ExecTool) Description() string { return t.def.Description }

// Definition implements plugin.Tool.
func (t *ExecTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        t.def.Name,
		Description: t.def.Description,
		Parameters:  t.def.Parameters,
	}
}

// Execute implements plugin.Tool. It marshals args to JSON, writes to stdin,
// and parses the binary's stdout as JSON.
func (t *ExecTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	input, err := json.Marshal(map[string]any{"name": t.def.Name, "arguments": args})
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}

	cmd := exec.CommandContext(ctx, t.binPath)
	cmd.Stdin = bytes.NewReader(input)

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			return nil, fmt.Errorf("tool %s exited with error: %w\nstderr: %s", t.def.Name, err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("run tool %s: %w", t.def.Name, err)
	}

	var result any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse tool output: %w", err)
	}
	return result, nil
}

// isExitError is a helper to avoid importing os/exec in tests.
func isExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
