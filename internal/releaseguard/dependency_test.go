package releaseguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	selected := selectedModuleGraph(t)
	for module, want := range required {
		version, present := selected[module]
		if !present {
			t.Errorf("selected module graph is missing %s", module)
			continue
		}
		if version != want.version {
			t.Errorf("selected %s version = %s, want %s", module, version, want.version)
		}
	}
	if version, present := selected[legacyDocker]; present {
		t.Errorf("selected module graph retains legacy Docker module %s", version)
	}

	protected := make(map[string]struct{}, len(required)+2)
	for module := range required {
		protected[module] = struct{}{}
	}
	protected[legacyDocker] = struct{}{}
	protected["github.com/GoCodeAlone/workflow-plugin-authz"] = struct{}{}
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

	for _, module := range []string{
		"github.com/moby/moby/api",
		"github.com/moby/moby/client",
		"github.com/ollama/ollama",
	} {
		why := moduleWhy(t, module)
		if !strings.Contains(why, "github.com/GoCodeAlone/workflow-plugin-agent/") {
			t.Errorf("%s ownership path does not cross workflow-plugin-agent:\n%s", module, why)
		}
	}
}

func selectedModuleGraph(t *testing.T) map[string]string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-mod=readonly", "-m", "-json", "all")
	cmd.Dir = repoRoot(t)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("list selected module graph: %v\n%s", err, stderr.String())
	}

	selected := make(map[string]string)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var module struct {
			Path    string
			Version string
		}
		err := decoder.Decode(&module)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode selected module graph: %v", err)
		}
		selected[module.Path] = module.Version
	}
	return selected
}

func moduleWhy(t *testing.T, module string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "mod", "why", "-m", module)
	cmd.Dir = repoRoot(t)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("explain module ownership for %s: %v\n%s", module, err, raw)
	}
	return string(raw)
}
