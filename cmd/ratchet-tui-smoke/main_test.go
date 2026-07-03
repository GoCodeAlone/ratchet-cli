package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSmokeBinaryBuildTags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke driver is Unix-only")
	}
	root := repoRoot(t)

	assertGoListHasNoGoFiles(t, root, nil, "go list ./cmd/ratchet-tui-smoke")
	assertGoFails(t, root, nil, "go build ./cmd/ratchet-tui-smoke", "build", "./cmd/ratchet-tui-smoke")

	out := filepath.Join(t.TempDir(), "ratchet-tui-smoke")
	assertGoSucceeds(t, root, nil, "go build -tags tui_smoke ./cmd/ratchet-tui-smoke",
		"build", "-tags", "tui_smoke", "-o", out, "./cmd/ratchet-tui-smoke")

	for _, arch := range []string{"amd64", "arm64"} {
		env := []string{"GOOS=windows", "GOARCH=" + arch}
		assertGoListHasNoGoFiles(t, root, env, "windows go list -tags tui_smoke "+arch, "-tags", "tui_smoke")
		assertGoFails(t, root, env, "windows go build -tags tui_smoke "+arch,
			"build", "-tags", "tui_smoke", "./cmd/ratchet-tui-smoke")
	}
}

func assertGoListHasNoGoFiles(t *testing.T, root string, env []string, label string, extraArgs ...string) {
	t.Helper()
	args := append([]string{"list"}, extraArgs...)
	args = append(args, "-f", "{{len .GoFiles}}", "./cmd/ratchet-tui-smoke")
	out, err := runGo(root, env, args...)
	if err != nil {
		t.Fatalf("%s: go list failed unexpectedly: %v\n%s", label, err, out)
	}
	if strings.TrimSpace(out) != "0" {
		t.Fatalf("%s: expected zero non-test buildable files, got %q", label, strings.TrimSpace(out))
	}
}

func assertGoSucceeds(t *testing.T, root string, env []string, label string, args ...string) {
	t.Helper()
	out, err := runGo(root, env, args...)
	if err != nil {
		t.Fatalf("%s: expected success, got %v\n%s", label, err, out)
	}
}

func assertGoFails(t *testing.T, root string, env []string, label string, args ...string) {
	t.Helper()
	out, err := runGo(root, env, args...)
	if err == nil {
		t.Fatalf("%s: expected failure, got success\n%s", label, out)
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "build constraints exclude all go files") &&
		!strings.Contains(lower, "no go files") &&
		!strings.Contains(lower, "no non-test go files") &&
		!strings.Contains(lower, "no buildable go source files") {
		t.Fatalf("%s: failure did not identify Unix-only/no-buildable package:\n%s", label, out)
	}
}

func runGo(root string, env []string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func repoRoot(t *testing.T) string {
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
