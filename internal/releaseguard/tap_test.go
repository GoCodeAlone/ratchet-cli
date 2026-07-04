package releaseguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoReleaserHomebrewCaskConfig(t *testing.T) {
	cfg, err := LoadGoReleaserConfig(filepath.Join(repoRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load goreleaser config: %v", err)
	}
	if err := ValidateHomebrewCaskConfig(cfg); err != nil {
		t.Fatalf("validate cask config: %v", err)
	}
}

func TestTapPreflight(t *testing.T) {
	if os.Getenv("RATCHET_RELEASE_GUARD_MODE") != "tap-preflight" {
		t.Skip("releaseguard artifact mode not requested")
	}
	if os.Getenv("RATCHET_RELEASE_GUARD_TAP") == "" {
		t.Fatal("RATCHET_RELEASE_GUARD_TAP is required")
	}
	if err := RunFromEnv(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTapPreflightRejectsLegacyInstallSurfaces(t *testing.T) {
	tap := t.TempDir()
	mustWrite(t, filepath.Join(tap, "ratchet-cli.rb"), `cask "ratchet-cli" do
  binary "ratchet"
end
`)
	mustWrite(t, filepath.Join(tap, "Formula", "ratchet-cli.rb"), "class RatchetCli < Formula\nend\n")
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
  binary "ratchet"
end
`)
	err := GuardTapPreflight(repoRoot(t), tap)
	if err == nil {
		t.Fatal("expected stale tap surfaces to fail")
	}
	for _, want := range []string{"ratchet-cli.rb", "Formula/ratchet-cli.rb"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

func TestTapPreflightAcceptsManagedCask(t *testing.T) {
	tap := t.TempDir()
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0/ratchet_darwin_arm64.tar.gz"
  name "ratchet-cli"
  binary "ratchet"
end
`)
	if err := GuardTapPreflight(repoRoot(t), tap); err != nil {
		t.Fatalf("guard tap preflight: %v", err)
	}
}

func TestTapPostcheckUsesNamesCommitsAndVersion(t *testing.T) {
	tap := t.TempDir()
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
  version "0.0.0-test"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0-test/ratchet_darwin_arm64.tar.gz"
  name "ratchet-cli"
  binary "ratchet"
end
`)
	root := repoRoot(t)
	if err := GuardTapPostcheck(root, tap, "other", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-test"); err == nil {
		t.Fatal("expected tap-postcheck names failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "README.md=fixture-sha", "v0.0.0-test"); err == nil {
		t.Fatal("expected tap-postcheck commits failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-other"); err == nil {
		t.Fatal("expected tap-postcheck version failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-test"); err != nil {
		t.Fatalf("tap postcheck: %v", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
