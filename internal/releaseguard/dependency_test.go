package releaseguard

import (
	"os"
	"path/filepath"
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
	for _, dependency := range mod.Require {
		if dependency.Mod.Path == legacyDocker {
			t.Errorf("go.mod retains legacy Docker module %s", dependency.Mod.Version)
		}
	}

	tracked := map[string]struct{}{
		"github.com/GoCodeAlone/workflow-plugin-agent": {},
		"github.com/docker/docker":                     {},
		"github.com/moby/moby/api":                     {},
		"github.com/moby/moby/client":                  {},
		"github.com/ollama/ollama":                     {},
		"google.golang.org/grpc":                       {},
	}
	for _, replacement := range mod.Replace {
		if _, forbidden := tracked[replacement.Old.Path]; forbidden {
			t.Errorf("go.mod replaces tracked module %s", replacement.Old.Path)
		}
	}
	for _, exclusion := range mod.Exclude {
		if _, forbidden := tracked[exclusion.Mod.Path]; forbidden {
			t.Errorf("go.mod excludes tracked module %s", exclusion.Mod.Path)
		}
	}
}
