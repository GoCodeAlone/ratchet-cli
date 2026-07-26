package releaseguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestSecurityDependencyOwnership(t *testing.T) {
	path := filepath.Join(repoRoot(t), "go.mod")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	mod, err := modfile.Parse(path, raw, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}

	type requirement struct {
		version  string
		indirect bool
	}
	required := map[string]requirement{
		"github.com/GoCodeAlone/workflow-plugin-agent": {version: "v0.12.10"},
		"github.com/moby/moby/api":                     {version: "v1.55.0", indirect: true},
		"github.com/moby/moby/client":                  {version: "v0.5.0", indirect: true},
		"github.com/ollama/ollama":                     {version: "v0.32.4", indirect: true},
		"google.golang.org/grpc":                       {version: "v1.82.1"},
	}

	found := make(map[string]*modfile.Require, len(required))
	for _, dependency := range mod.Require {
		if _, tracked := required[dependency.Mod.Path]; tracked {
			found[dependency.Mod.Path] = dependency
		}
	}
	for module, want := range required {
		dependency := found[module]
		if dependency == nil {
			t.Errorf("go.mod is missing required module %s", module)
			continue
		}
		if dependency.Mod.Version != want.version {
			t.Errorf("%s version = %s, want %s", module, dependency.Mod.Version, want.version)
		}
		if dependency.Indirect != want.indirect {
			t.Errorf("%s indirect = %t, want %t", module, dependency.Indirect, want.indirect)
		}
	}

	const legacyDocker = "github.com/docker/docker"
	protected := make(map[string]struct{}, len(required)+2)
	for module := range required {
		protected[module] = struct{}{}
	}
	protected[legacyDocker] = struct{}{}
	protected["github.com/GoCodeAlone/workflow-plugin-authz"] = struct{}{}

	selected := selectedModuleGraph(t)
	for module, want := range required {
		dependency, present := selected[module]
		if !present {
			t.Errorf("selected module graph is missing %s", module)
			continue
		}
		if dependency.Version != want.version {
			t.Errorf("selected %s version = %s, want %s", module, dependency.Version, want.version)
		}
	}
	for _, replacement := range mod.Replace {
		if _, forbidden := protected[replacement.Old.Path]; forbidden {
			t.Errorf("go.mod replaces tracked module %s", replacement.Old.Path)
		}
	}
	for _, exclusion := range mod.Exclude {
		if _, forbidden := protected[exclusion.Mod.Path]; forbidden {
			t.Errorf("go.mod excludes tracked module %s", exclusion.Mod.Path)
		}
	}
	for _, violation := range selectedModuleGraphViolations(selected, protected, legacyDocker) {
		t.Error(violation)
	}

	for _, target := range []releaseTarget{
		{goos: "darwin", goarch: "amd64"},
		{goos: "darwin", goarch: "arm64"},
		{goos: "linux", goarch: "amd64"},
		{goos: "linux", goarch: "arm64"},
		{goos: "windows", goarch: "amd64"},
		{goos: "windows", goarch: "arm64"},
	} {
		t.Run(target.goos+"_"+target.goarch, func(t *testing.T) {
			graph := productionPackageGraph(t, target)
			for _, dependency := range []struct {
				module string
				root   string
			}{
				{module: "github.com/moby/moby/api", root: "github.com/GoCodeAlone/ratchet-cli/internal/daemon"},
				{module: "github.com/moby/moby/client", root: "github.com/GoCodeAlone/ratchet-cli/internal/daemon"},
				{module: "github.com/ollama/ollama", root: "github.com/GoCodeAlone/ratchet-cli/cmd/ratchet"},
			} {
				if !productionModuleOwnedByAgent(graph, dependency.root, dependency.module) {
					t.Errorf("production import graph has a missing or bypassing %s -> workflow-plugin-agent -> %s path", dependency.root, dependency.module)
				}
			}
		})
	}
}

type selectedModule struct {
	Path    string
	Version string
	Replace *selectedModule
}

func selectedModuleGraph(t *testing.T) map[string]selectedModule {
	t.Helper()
	cmd := isolatedGoCommand(t, "list", "-mod=readonly", "-m", "-json", "all")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("list selected module graph: %v\n%s", err, stderr.String())
	}

	selected := make(map[string]selectedModule)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var module selectedModule
		err := decoder.Decode(&module)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode selected module graph: %v", err)
		}
		selected[module.Path] = module
	}
	return selected
}

type productionPackage struct {
	ImportPath string
	Imports    []string
	Module     *selectedModule
}

type releaseTarget struct {
	goos   string
	goarch string
}

func productionPackageGraph(t *testing.T, target releaseTarget) map[string]productionPackage {
	t.Helper()
	cmd := isolatedGoCommand(t, "list", "-mod=readonly", "-deps", "-json", "./internal/daemon", "./cmd/ratchet")
	setCommandEnvironment(cmd,
		"GOOS="+target.goos,
		"GOARCH="+target.goarch,
		"CGO_ENABLED=0",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("list production package graph: %v\n%s", err, stderr.String())
	}

	graph := make(map[string]productionPackage)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var pkg productionPackage
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode production package graph: %v", err)
		}
		graph[pkg.ImportPath] = pkg
	}
	return graph
}

func productionModuleOwnedByAgent(graph map[string]productionPackage, root, targetModule string) bool {
	const agentModule = "github.com/GoCodeAlone/workflow-plugin-agent"
	type state struct {
		importPath   string
		crossesAgent bool
	}
	queue := []state{{importPath: root}}
	seen := make(map[state]struct{})
	var owned, bypassed bool
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, visited := seen[current]; visited {
			continue
		}
		seen[current] = struct{}{}

		pkg, present := graph[current.importPath]
		if !present {
			continue
		}
		current.crossesAgent = current.crossesAgent || (pkg.Module != nil && pkg.Module.Path == agentModule)
		if pkg.Module != nil && pkg.Module.Path == targetModule {
			if current.crossesAgent {
				owned = true
			} else {
				bypassed = true
			}
			continue
		}
		for _, imported := range pkg.Imports {
			queue = append(queue, state{importPath: imported, crossesAgent: current.crossesAgent})
		}
	}
	return owned && !bypassed
}

func TestProductionModuleOwnedByAgent(t *testing.T) {
	const (
		root          = "github.com/GoCodeAlone/ratchet-cli/internal/daemon"
		agent         = "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
		target        = "github.com/moby/moby/client"
		targetPackage = target
	)
	targetNode := productionPackage{
		ImportPath: targetPackage,
		Module:     &selectedModule{Path: target},
	}
	tests := []struct {
		name  string
		graph map[string]productionPackage
		want  bool
	}{
		{
			name: "owner path",
			graph: map[string]productionPackage{
				root:          {ImportPath: root, Imports: []string{agent}},
				agent:         {ImportPath: agent, Imports: []string{targetPackage}, Module: &selectedModule{Path: "github.com/GoCodeAlone/workflow-plugin-agent"}},
				targetPackage: targetNode,
			},
			want: true,
		},
		{
			name: "owner and direct paths",
			graph: map[string]productionPackage{
				root:          {ImportPath: root, Imports: []string{agent, targetPackage}},
				agent:         {ImportPath: agent, Imports: []string{targetPackage}, Module: &selectedModule{Path: "github.com/GoCodeAlone/workflow-plugin-agent"}},
				targetPackage: targetNode,
			},
		},
		{
			name: "owner root package",
			graph: map[string]productionPackage{
				root: {
					ImportPath: root,
					Imports:    []string{"github.com/GoCodeAlone/workflow-plugin-agent"},
				},
				"github.com/GoCodeAlone/workflow-plugin-agent": {
					ImportPath: "github.com/GoCodeAlone/workflow-plugin-agent",
					Imports:    []string{targetPackage},
					Module:     &selectedModule{Path: "github.com/GoCodeAlone/workflow-plugin-agent"},
				},
				targetPackage: targetNode,
			},
			want: true,
		},
		{
			name: "direct non-owner path",
			graph: map[string]productionPackage{
				root:          {ImportPath: root, Imports: []string{targetPackage}},
				targetPackage: targetNode,
			},
		},
		{
			name: "unreachable test-only owner",
			graph: map[string]productionPackage{
				root:          {ImportPath: root},
				agent:         {ImportPath: agent, Imports: []string{targetPackage}, Module: &selectedModule{Path: "github.com/GoCodeAlone/workflow-plugin-agent"}},
				targetPackage: targetNode,
			},
		},
		{
			name: "non-owner cycle",
			graph: map[string]productionPackage{
				root:                {ImportPath: root, Imports: []string{"example.com/cycle"}},
				"example.com/cycle": {ImportPath: "example.com/cycle", Imports: []string{root}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := productionModuleOwnedByAgent(test.graph, root, target); got != test.want {
				t.Fatalf("productionModuleOwnedByAgent() = %t, want %t", got, test.want)
			}
		})
	}
}

func selectedModuleGraphViolations(selected map[string]selectedModule, protected map[string]struct{}, legacy string) []string {
	var violations []string
	for _, dependency := range selected {
		if dependency.Path == legacy {
			violations = append(violations, fmt.Sprintf("selected module graph retains legacy Docker module %s", dependency.Version))
		}
		if dependency.Replace == nil {
			continue
		}
		if _, forbidden := protected[dependency.Path]; forbidden {
			violations = append(violations, fmt.Sprintf("selected module graph replaces tracked module %s with %s", dependency.Path, dependency.Replace.Path))
		}
		if _, forbidden := protected[dependency.Replace.Path]; forbidden {
			violations = append(violations, fmt.Sprintf("selected module graph uses protected module %s as a replacement for %s", dependency.Replace.Path, dependency.Path))
		}
	}
	return violations
}

func TestSelectedModuleGraphViolations(t *testing.T) {
	const (
		protectedModule = "github.com/moby/moby/client"
		legacyDocker    = "github.com/docker/docker"
	)
	protected := map[string]struct{}{
		protectedModule: {},
		legacyDocker:    {},
	}
	tests := []struct {
		name          string
		selected      map[string]selectedModule
		wantViolation bool
	}{
		{
			name: "clean graph",
			selected: map[string]selectedModule{
				protectedModule: {Path: protectedModule, Version: "v0.5.0"},
			},
		},
		{
			name: "legacy requested path",
			selected: map[string]selectedModule{
				legacyDocker: {Path: legacyDocker, Version: "v28.5.2+incompatible"},
			},
			wantViolation: true,
		},
		{
			name: "protected requested replacement",
			selected: map[string]selectedModule{
				protectedModule: {
					Path:    protectedModule,
					Version: "v0.5.0",
					Replace: &selectedModule{Path: "example.com/fork", Version: "v1.0.0"},
				},
			},
			wantViolation: true,
		},
		{
			name: "protected effective replacement",
			selected: map[string]selectedModule{
				"example.com/dependency": {
					Path:    "example.com/dependency",
					Version: "v1.0.0",
					Replace: &selectedModule{Path: protectedModule, Version: "v0.5.0"},
				},
			},
			wantViolation: true,
		},
		{
			name: "legacy effective replacement",
			selected: map[string]selectedModule{
				"example.com/dependency": {
					Path:    "example.com/dependency",
					Version: "v1.0.0",
					Replace: &selectedModule{Path: legacyDocker, Version: "v28.5.2+incompatible"},
				},
			},
			wantViolation: true,
		},
		{
			name: "benign replacement",
			selected: map[string]selectedModule{
				"example.com/dependency": {
					Path:    "example.com/dependency",
					Version: "v1.0.0",
					Replace: &selectedModule{Path: "example.com/fork", Version: "v1.0.1"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotViolation := len(selectedModuleGraphViolations(test.selected, protected, legacyDocker)) > 0
			if gotViolation != test.wantViolation {
				t.Fatalf("violation = %t, want %t", gotViolation, test.wantViolation)
			}
		})
	}
}

func TestIsolatedGoCommandEnvironment(t *testing.T) {
	t.Setenv("GOWORK", "/tmp/untrusted-go.work")
	t.Setenv("GOFLAGS", "-mod=mod")

	cmd := isolatedGoCommand(t, "version")
	values := make(map[string][]string)
	for _, entry := range cmd.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && (key == "GOWORK" || key == "GOFLAGS") {
			values[key] = append(values[key], value)
		}
	}
	if got := values["GOWORK"]; len(got) != 1 || got[0] != "off" {
		t.Fatalf("GOWORK values = %q, want [off]", got)
	}
	if got := values["GOFLAGS"]; len(got) != 1 || got[0] != "" {
		t.Fatalf("GOFLAGS values = %q, want one empty value", got)
	}
}

func isolatedGoCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = repoRoot(t)
	setCommandEnvironment(cmd, "GOWORK=off", "GOFLAGS=")
	return cmd
}

func setCommandEnvironment(cmd *exec.Cmd, overrides ...string) {
	keys := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		keys[key] = struct{}{}
	}
	cmd.Env = slices.DeleteFunc(cmd.Environ(), func(entry string) bool {
		key, _, _ := strings.Cut(entry, "=")
		_, overridden := keys[key]
		return overridden
	})
	cmd.Env = append(cmd.Env, overrides...)
}
