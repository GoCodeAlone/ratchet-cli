package releaseguard

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoReleaserConfigDraftAndTaxonomy(t *testing.T) {
	cfg, err := LoadGoReleaserConfig(filepath.Join(repoRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load goreleaser config: %v", err)
	}
	if err := ValidateGoReleaserConfig(cfg); err != nil {
		t.Fatalf("validate goreleaser config: %v", err)
	}
}

func TestGoReleaserReleaseDraftConfig(t *testing.T) {
	cfg, err := LoadGoReleaserConfig(filepath.Join(repoRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("load goreleaser config: %v", err)
	}
	if !cfg.Release.Draft {
		t.Fatal("release preflight requires .goreleaser.yaml release.draft: true")
	}
	if err := ValidateGoReleaserConfig(cfg); err != nil {
		t.Fatalf("validate draft release config: %v", err)
	}
}

func TestGoReleaserConfigRejectsUnknownPublishableKeys(t *testing.T) {
	cfg := GoReleaserConfig{
		RawTopLevel: map[string]any{
			"version":  2,
			"builds":   []any{},
			"archives": []any{},
			"release":  map[string]any{"draft": true},
			"brews":    []any{},
		},
		Release: GoReleaserRelease{Draft: true},
	}
	err := ValidateGoReleaserConfig(cfg)
	if err == nil {
		t.Fatal("expected deprecated brews key to fail")
	}
	if !strings.Contains(err.Error(), "brews") {
		t.Fatalf("error %q does not name brews", err)
	}
}

func TestReleaseGuardModeNotRequested(t *testing.T) {
	t.Setenv("RATCHET_RELEASE_GUARD_MODE", "")
	if err := RunFromEnv(repoRoot(t)); !errors.Is(err, ErrArtifactModeNotRequested) {
		t.Fatalf("RunFromEnv without mode error = %v, want ErrArtifactModeNotRequested", err)
	}
}

func TestManifestGuardRequiresDistEnv(t *testing.T) {
	t.Setenv("RATCHET_RELEASE_GUARD_MODE", "manifest")
	t.Setenv("RATCHET_RELEASE_GUARD_DIST", "")
	err := RunFromEnv(repoRoot(t))
	if err == nil {
		t.Fatal("expected missing RATCHET_RELEASE_GUARD_DIST failure")
	}
	if !strings.Contains(err.Error(), "RATCHET_RELEASE_GUARD_DIST") {
		t.Fatalf("error %q does not name RATCHET_RELEASE_GUARD_DIST", err)
	}
}

func TestManifestGuard(t *testing.T) {
	if os.Getenv("RATCHET_RELEASE_GUARD_MODE") != "manifest" {
		t.Skip("releaseguard artifact mode not requested")
	}
	if os.Getenv("RATCHET_RELEASE_GUARD_DIST") == "" {
		t.Fatal("RATCHET_RELEASE_GUARD_DIST is required")
	}
	if err := RunFromEnv(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestManifestGuardRejectsSmokeTokens(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
		extraMember: "ratchet-tui-smoke",
	})
	err := GuardManifest(repoRoot(t), dist)
	if err == nil {
		t.Fatal("expected smoke token failure")
	}
	if !strings.Contains(err.Error(), "ratchet-tui-smoke") {
		t.Fatalf("error %q does not name smoke token", err)
	}
}

func TestManifestGuardRejectsArchiveMissingPackagedBinary(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	target := nonHostTarget()
	path := filepath.Join(dist, target.archiveName())
	if target.goos == "windows" {
		writeZipWithoutBinary(t, path)
	} else {
		writeTarWithoutBinary(t, path)
	}
	rewriteChecksum(t, dist, target.archiveName())

	err := GuardManifest(repoRoot(t), dist)
	if err == nil {
		t.Fatal("expected missing packaged binary failure")
	}
	want := "missing packaged ratchet binary"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func TestManifestGuardRejectsArchiveDuplicatePackagedBinary(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	target := nonHostTarget()
	path := filepath.Join(dist, target.archiveName())
	if target.goos == "windows" {
		writeZipWithDuplicateBinary(t, path)
	} else {
		writeTarWithDuplicateBinary(t, path)
	}
	rewriteChecksum(t, dist, target.archiveName())

	err := GuardManifest(repoRoot(t), dist)
	if err == nil {
		t.Fatal("expected duplicate packaged binary failure")
	}
	if strings.Contains(err.Error(), "missing packaged ratchet binary") {
		t.Fatalf("duplicate binary error should not say missing: %q", err)
	}
	if !strings.Contains(err.Error(), "has 2 packaged ratchet binaries") {
		t.Fatalf("error %q does not name duplicate packaged binaries", err)
	}
}

func TestManifestGuardRequiresGeneratedCaskBinaryDirective(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	if err := os.WriteFile(filepath.Join(dist, "ratchet-cli.rb"), []byte(`cask "ratchet-cli" do
  name "ratchet-cli"
end
`), 0o644); err != nil {
		t.Fatalf("write cask without binary: %v", err)
	}
	err := GuardManifest(repoRoot(t), dist)
	if err == nil {
		t.Fatal("expected missing generated cask binary directive failure")
	}
	if !strings.Contains(err.Error(), `binary "ratchet"`) {
		t.Fatalf("error %q does not name missing binary directive", err)
	}
}

func TestDraftAssets(t *testing.T) {
	if os.Getenv("RATCHET_RELEASE_GUARD_MODE") != "draft-assets" {
		t.Skip("releaseguard draft-assets mode not requested")
	}
	assets := os.Getenv("RATCHET_RELEASE_GUARD_ASSETS")
	if assets == "" {
		t.Fatal("RATCHET_RELEASE_GUARD_ASSETS is required")
	}
	if os.Getenv("RATCHET_RELEASE_GUARD_VERSION") == "" {
		t.Fatal("RATCHET_RELEASE_GUARD_VERSION is required")
	}
	prepareModeFixtureDist(t, assets, true)
	if err := RunFromEnv(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestDraftAssetsRequiresMatchingVersion(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), []byte(`{"tag":"v0.0.0-other","version":"0.0.0-other"}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	err := GuardDraftAssets(repoRoot(t), dist, "v0.0.0-test")
	if err == nil {
		t.Fatal("expected draft asset version mismatch")
	}
	if !strings.Contains(err.Error(), "v0.0.0-test") {
		t.Fatalf("error %q does not name requested version", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), []byte(`{"tag":"v0.0.0-test","version":"0.0.0-test","draft":true}`), 0o644); err != nil {
		t.Fatalf("write matching metadata: %v", err)
	}
	if err := GuardDraftAssets(repoRoot(t), dist, "v0.0.0-test"); err != nil {
		t.Fatalf("matching draft assets: %v", err)
	}
}

func TestDraftAssetsRequiresDraftMetadata(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), []byte(`{"tag":"v0.0.0-test","version":"0.0.0-test"}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	err := GuardDraftAssets(repoRoot(t), dist, "v0.0.0-test")
	if err == nil {
		t.Fatal("expected missing draft metadata to fail")
	}
	if !strings.Contains(err.Error(), "draft") {
		t.Fatalf("error %q does not name draft state", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), []byte(`{"tag":"v0.0.0-test","version":"0.0.0-test","draft":false}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	err = GuardDraftAssets(repoRoot(t), dist, "v0.0.0-test")
	if err == nil {
		t.Fatal("expected non-draft metadata to fail")
	}
	if !strings.Contains(err.Error(), "draft") {
		t.Fatalf("error %q does not name draft state", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), []byte(`{"tag":"v0.0.0-test","version":"0.0.0-test","draft":true}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := GuardDraftAssets(repoRoot(t), dist, "v0.0.0-test"); err != nil {
		t.Fatalf("draft metadata should pass: %v", err)
	}
}

func TestWindowsArchiveRequiresBothZips(t *testing.T) {
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	missing := "ratchet_windows_arm64.zip"
	if err := os.Remove(filepath.Join(dist, missing)); err != nil {
		t.Fatalf("remove windows archive fixture: %v", err)
	}
	err := GuardManifest(repoRoot(t), dist)
	if err == nil {
		t.Fatal("expected missing windows archive failure")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error %q does not name missing windows archive %s", err, missing)
	}
}

func TestWindowsArchive(t *testing.T) {
	if os.Getenv("RATCHET_RELEASE_GUARD_MODE") != "manifest" {
		t.Skip("releaseguard manifest mode not requested")
	}
	dist := os.Getenv("RATCHET_RELEASE_GUARD_DIST")
	if dist == "" {
		t.Fatal("RATCHET_RELEASE_GUARD_DIST is required")
	}
	prepareModeFixtureDist(t, dist, false)
	if err := RunFromEnv(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestManifestGuardAcceptsCompleteDistAndRunsHostBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host binary execution fixture is Unix-only")
	}
	dist := t.TempDir()
	writeMatrixDist(t, dist, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	if err := GuardManifest(repoRoot(t), dist); err != nil {
		t.Fatalf("guard complete dist: %v", err)
	}
}

func nonHostTarget() archiveTarget {
	for _, target := range archiveTargets() {
		if target.goos != runtime.GOOS || target.goarch != runtime.GOARCH {
			return target
		}
	}
	return archiveTarget{goos: "windows", goarch: "amd64"}
}

type archivePayload struct {
	hostVersion string
	hostHelp    string
	extraMember string
}

func writeMatrixDist(t *testing.T, dist string, payload archivePayload) {
	t.Helper()
	var checksums []string
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("ratchet_%s_%s", goos, goarch)
			archiveName := name + ".tar.gz"
			if goos == "windows" {
				archiveName = name + ".zip"
			}
			path := filepath.Join(dist, archiveName)
			if goos == "windows" {
				writeZipArchive(t, path, goos, goarch, payload)
			} else {
				writeTarArchive(t, path, goos, goarch, payload)
			}
			sum := sha256File(t, path)
			checksums = append(checksums, fmt.Sprintf("%s  %s", sum, archiveName))
		}
	}
	checksums = append(checksums, "")
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(strings.Join(checksums, "\n")), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "ratchet-cli.rb"), []byte(`cask "ratchet-cli" do
  binary "ratchet"
end
`), 0o644); err != nil {
		t.Fatalf("write cask: %v", err)
	}
}

func writeTarWithoutBinary(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar fixture: %v", err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	writeTarFile(t, tw, "RATCHET.md", "approved docs\n", 0o644)
}

func writeTarWithDuplicateBinary(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar fixture: %v", err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	writeTarFile(t, tw, "ratchet", "release binary\n", 0o755)
	writeTarFile(t, tw, "bin/ratchet", "release binary\n", 0o755)
}

func writeZipWithoutBinary(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip fixture: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	writeZipFile(t, zw, "RATCHET.md", "approved docs\n")
}

func writeZipWithDuplicateBinary(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip fixture: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	writeZipFile(t, zw, "ratchet.exe", "release binary\n")
	writeZipFile(t, zw, "bin/ratchet.exe", "release binary\n")
}

func rewriteChecksum(t *testing.T, dist, archiveName string) {
	t.Helper()
	checksumPath := filepath.Join(dist, "checksums.txt")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			lines = append(lines, fmt.Sprintf("%s  %s", sha256File(t, filepath.Join(dist, archiveName)), archiveName))
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	lines = append(lines, "")
	if err := os.WriteFile(checksumPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
}

func writeTarArchive(t *testing.T, path, goos, goarch string, payload archivePayload) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar fixture: %v", err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	writeTarFile(t, tw, "ratchet", executablePayload(goos, goarch, payload), 0o755)
	if payload.extraMember != "" {
		writeTarFile(t, tw, payload.extraMember, "forbidden\n", 0o644)
	}
}

func writeTarFile(t *testing.T, tw *tar.Writer, name, body string, mode int64) {
	t.Helper()
	h := &tar.Header{Name: name, Mode: mode, Size: int64(len(body))}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := io.WriteString(tw, body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
}

func writeZipArchive(t *testing.T, path, goos, goarch string, payload archivePayload) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip fixture: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	writeZipFile(t, zw, "ratchet.exe", executablePayload(goos, goarch, payload))
	if payload.extraMember != "" {
		writeZipFile(t, zw, payload.extraMember, "forbidden\n")
	}
}

func writeZipFile(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip member: %v", err)
	}
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write zip member: %v", err)
	}
}

func executablePayload(goos, goarch string, payload archivePayload) string {
	if goos == runtime.GOOS && goarch == runtime.GOARCH && goos != "windows" {
		return fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\nversion) printf %q ;;\nhelp) printf %q ;;\n*) exit 2 ;;\nesac\n", payload.hostVersion, payload.hostHelp)
	}
	return "ratchet release binary\n"
}

func prepareModeFixtureDist(t *testing.T, path string, draft bool) {
	t.Helper()
	if !isReleaseGuardTestdataPath(path) {
		return
	}
	root := repoRoot(t)
	fixturePath := resolvePath(root, path)
	if err := os.RemoveAll(fixturePath); err != nil {
		t.Fatalf("clear fixture dist %s: %v", fixturePath, err)
	}
	if err := os.MkdirAll(fixturePath, 0o755); err != nil {
		t.Fatalf("create fixture dist %s: %v", fixturePath, err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixturePath)
	})
	writeMatrixDist(t, fixturePath, archivePayload{
		hostVersion: "ratchet version v0.0.0-test\n",
		hostHelp:    "ratchet help\n",
	})
	if draft {
		if err := os.WriteFile(filepath.Join(fixturePath, "metadata.json"), []byte(`{"tag":"v0.0.0-test","version":"0.0.0-test","draft":true}`), 0o644); err != nil {
			t.Fatalf("write draft fixture metadata: %v", err)
		}
	}
}

func isReleaseGuardTestdataPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "internal/releaseguard/testdata/")
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open for checksum: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash file: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
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
