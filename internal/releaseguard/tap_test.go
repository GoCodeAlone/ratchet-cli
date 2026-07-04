package releaseguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestPublishHomebrewCaskScriptCommitsGeneratedCask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script guard is verified on Unix CI")
	}
	root := repoRoot(t)
	tap := t.TempDir()
	generated := filepath.Join(t.TempDir(), "ratchet-cli.rb")
	envFile := filepath.Join(t.TempDir(), "github-env")

	runGit(t, tap, "init", "-b", "main")
	runGit(t, tap, "config", "user.name", "test")
	runGit(t, tap, "config", "user.email", "test@example.invalid")
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
  version "0.0.0"
  binary "ratchet"
end
`)
	runGit(t, tap, "add", "Casks/ratchet-cli.rb")
	runGit(t, tap, "commit", "-m", "seed cask")

	mustWrite(t, generated, `cask "ratchet-cli" do
  version "0.0.1"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.1/ratchet_darwin_arm64.tar.gz"
  name "ratchet-cli"
  binary "ratchet"
end
`)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "publish-homebrew-cask.sh"), generated, tap)
	cmd.Env = append(os.Environ(), "GITHUB_ENV="+envFile, "RELEASE_TAG=v0.0.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("publish cask script: %v\n%s", err, out)
	}
	envData, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read github env: %v", err)
	}
	if !strings.Contains(string(envData), "RATCHET_RELEASE_GUARD_TAP_COMMITS=Casks/ratchet-cli.rb=") {
		t.Fatalf("github env missing tap commit: %s", envData)
	}
	data, err := os.ReadFile(filepath.Join(tap, "Casks", "ratchet-cli.rb"))
	if err != nil {
		t.Fatalf("read published cask: %v", err)
	}
	if !strings.Contains(string(data), `version "0.0.1"`) {
		t.Fatalf("published cask missing generated version:\n%s", data)
	}
	runGit(t, tap, "diff", "--quiet")
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
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=", "v0.0.0-test"); err == nil {
		t.Fatal("expected tap-postcheck empty cask commit failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-other"); err == nil {
		t.Fatal("expected tap-postcheck version failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-test"); err != nil {
		t.Fatalf("tap postcheck: %v", err)
	}
}

func TestTapPostcheck(t *testing.T) {
	if os.Getenv("RATCHET_RELEASE_GUARD_MODE") != "tap-postcheck" {
		t.Skip("releaseguard tap-postcheck mode not requested")
	}
	for _, name := range []string{
		"RATCHET_RELEASE_GUARD_TAP",
		"RATCHET_RELEASE_GUARD_TAP_NAMES",
		"RATCHET_RELEASE_GUARD_TAP_COMMITS",
		"RATCHET_RELEASE_GUARD_VERSION",
	} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required", name)
		}
	}
	prepareModeFixtureTap(t, os.Getenv("RATCHET_RELEASE_GUARD_TAP"), os.Getenv("RATCHET_RELEASE_GUARD_VERSION"))
	if err := RunFromEnv(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func prepareModeFixtureTap(t *testing.T, path, version string) {
	t.Helper()
	if !isReleaseGuardTestdataPath(path) {
		return
	}
	root := repoRoot(t)
	fixturePath := resolvePath(root, path)
	if err := os.RemoveAll(fixturePath); err != nil {
		t.Fatalf("clear fixture tap %s: %v", fixturePath, err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixturePath)
	})
	mustWrite(t, filepath.Join(fixturePath, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
  version "`+strings.TrimPrefix(version, "v")+`"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/`+version+`/ratchet_darwin_arm64.tar.gz"
  name "ratchet-cli"
  binary "ratchet"
end
`)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
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
