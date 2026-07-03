package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var smokeSourceManifest = map[string]struct{}{
	"cmd/ratchet-tui-smoke/main.go":        {},
	"internal/client/client_tui_smoke.go":  {},
	"internal/daemon/service_tui_smoke.go": {},
}

func TestSmokeSourceManifest(t *testing.T) {
	root := tuiRepoRoot(t)
	for rel := range smokeSourceManifest {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read manifest entry %s: %v", rel, err)
		}
		if !strings.HasPrefix(string(src), "//go:build tui_smoke && !windows\n\n") {
			t.Fatalf("%s must start with exact smoke build tag", rel)
		}
	}

	forbidden := []string{"ratchet-tui-smoke", "tui_smoke", "ConnectSmokeUnix"}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := smokeSourceManifest[rel]; ok {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range forbidden {
			if strings.Contains(string(src), token) {
				t.Fatalf("unmanifested non-test Go file %s contains smoke token %q", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
}

func tuiRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
