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

func TestGoReleaserRejectsDeprecatedBrewSurface(t *testing.T) {
	cfg, err := LoadGoReleaserConfig(filepath.Join(repoRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load goreleaser config: %v", err)
	}
	if _, ok := cfg.RawTopLevel["brews"]; !ok {
		cfg.RawTopLevel["brews"] = []any{map[string]any{"name": "ratchet-cli"}}
	}
	err = ValidateGoReleaserConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "deprecated publish surface") {
		t.Fatalf("expected deprecated brews rejection, got %v", err)
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
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
	  binary "ratchet"
	end
	`)
	mustWrite(t, filepath.Join(tap, "Formula", "ratchet-cli.rb"), `class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0/ratchet_darwin_arm64.tar.gz"
  sha256 "fixture"

  def install
    bin.install "ratchet"
  end
end
`)
	err := GuardTapPreflight(repoRoot(t), tap)
	if err == nil {
		t.Fatal("expected stale tap surfaces to fail")
	}
	if !strings.Contains(err.Error(), "ratchet-cli.rb") {
		t.Fatalf("error %q does not name stale root ratchet-cli.rb", err)
	}
}

func TestTapPreflightAcceptsManagedCaskAndFormula(t *testing.T) {
	tap := t.TempDir()
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
	  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0/ratchet_darwin_arm64.tar.gz"
	  name "ratchet-cli"
	  binary "ratchet"
	end
	`)
	mustWrite(t, filepath.Join(tap, "Formula", "ratchet-cli.rb"), `class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0/ratchet_darwin_arm64.tar.gz"
  sha256 "fixture"

  def install
    bin.install "ratchet"
  end
end
`)
	if err := GuardTapPreflight(repoRoot(t), tap); err != nil {
		t.Fatalf("guard tap preflight: %v", err)
	}
}

func TestPublishHomebrewScriptCommitsGeneratedCaskAndFormula(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script guard is verified on Unix CI")
	}
	root := repoRoot(t)
	tap := t.TempDir()
	generatedCask := filepath.Join(t.TempDir(), "ratchet-cli.rb")
	generatedFormula := filepath.Join(t.TempDir(), "ratchet-cli.rb")
	envFile := filepath.Join(t.TempDir(), "github-env")

	runGit(t, tap, "init", "-b", "main")
	runGit(t, tap, "config", "user.name", "test")
	runGit(t, tap, "config", "user.email", "test@example.invalid")
	mustWrite(t, filepath.Join(tap, "Casks", "ratchet-cli.rb"), `cask "ratchet-cli" do
	  version "0.0.0"
	  binary "ratchet"
	end
	`)
	mustWrite(t, filepath.Join(tap, "Formula", "ratchet-cli.rb"), `class RatchetCli < Formula
  def install
    bin.install "ratchet"
  end
end
`)
	runGit(t, tap, "add", "Casks/ratchet-cli.rb", "Formula/ratchet-cli.rb")
	runGit(t, tap, "commit", "-m", "seed homebrew files")

	mustWrite(t, generatedCask, `cask "ratchet-cli" do
	  version "0.0.1"
	  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.1/ratchet_darwin_arm64.tar.gz"
	  name "ratchet-cli"
	  binary "ratchet"
	end
	`)
	mustWrite(t, generatedFormula, `class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  version "0.0.1"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.1/ratchet_darwin_arm64.tar.gz"
  sha256 "fixture"

  def install
    bin.install "ratchet"
  end
end
`)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "publish-homebrew-cask.sh"), generatedCask, generatedFormula, tap)
	cmd.Env = append(os.Environ(), "GITHUB_ENV="+envFile, "RELEASE_TAG=v0.0.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("publish homebrew script: %v\n%s", err, out)
	}
	envData, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read github env: %v", err)
	}
	if !strings.Contains(string(envData), "RATCHET_RELEASE_GUARD_TAP_COMMITS=Casks/ratchet-cli.rb=") {
		t.Fatalf("github env missing tap commit: %s", envData)
	}
	if !strings.Contains(string(envData), ",Formula/ratchet-cli.rb=") {
		t.Fatalf("github env missing formula commit: %s", envData)
	}
	data, err := os.ReadFile(filepath.Join(tap, "Casks", "ratchet-cli.rb"))
	if err != nil {
		t.Fatalf("read published cask: %v", err)
	}
	if !strings.Contains(string(data), `version "0.0.1"`) {
		t.Fatalf("published cask missing generated version:\n%s", data)
	}
	data, err = os.ReadFile(filepath.Join(tap, "Formula", "ratchet-cli.rb"))
	if err != nil {
		t.Fatalf("read published formula: %v", err)
	}
	if !strings.Contains(string(data), `version "0.0.1"`) {
		t.Fatalf("published formula missing generated version:\n%s", data)
	}
	runGit(t, tap, "diff", "--quiet")
}

func TestRenderHomebrewFormulaScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script guard is verified on Unix CI")
	}
	root := repoRoot(t)
	dist := t.TempDir()
	out := filepath.Join(t.TempDir(), "ratchet-cli.rb")
	mustWrite(t, filepath.Join(dist, "checksums.txt"), `1111111111111111111111111111111111111111111111111111111111111111  ratchet_darwin_amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222  ratchet_darwin_arm64.tar.gz
3333333333333333333333333333333333333333333333333333333333333333  ratchet_linux_amd64.tar.gz
4444444444444444444444444444444444444444444444444444444444444444  ratchet_linux_arm64.tar.gz
`)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "render-homebrew-formula.sh"), dist, out)
	cmd.Env = append(os.Environ(), "RELEASE_TAG=v0.0.1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render formula: %v\n%s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read formula: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`class RatchetCli < Formula`,
		`version "0.0.1"`,
		`ratchet_darwin_arm64.tar.gz`,
		`sha256 "2222222222222222222222222222222222222222222222222222222222222222"`,
		`ratchet_linux_amd64.tar.gz`,
		`bin.install "ratchet"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formula missing %s:\n%s", want, text)
		}
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
	mustWrite(t, filepath.Join(tap, "Formula", "ratchet-cli.rb"), `class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  version "0.0.0-test"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/v0.0.0-test/ratchet_darwin_arm64.tar.gz"
  sha256 "fixture"

  def install
    bin.install "ratchet"
  end
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
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-test"); err == nil {
		t.Fatal("expected tap-postcheck formula commit failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha", "v0.0.0-other"); err == nil {
		t.Fatal("expected tap-postcheck version failure")
	}
	if err := GuardTapPostcheck(root, tap, "ratchet-cli", "Casks/ratchet-cli.rb=fixture-sha,Formula/ratchet-cli.rb=fixture-sha", "v0.0.0-test"); err != nil {
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
	mustWrite(t, filepath.Join(fixturePath, "Formula", "ratchet-cli.rb"), `class RatchetCli < Formula
  desc "Interactive AI agent CLI"
  homepage "https://github.com/GoCodeAlone/ratchet-cli"
  version "`+strings.TrimPrefix(version, "v")+`"
  url "https://github.com/GoCodeAlone/ratchet-cli/releases/download/`+version+`/ratchet_darwin_arm64.tar.gz"
  sha256 "fixture"

  def install
    bin.install "ratchet"
  end
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
