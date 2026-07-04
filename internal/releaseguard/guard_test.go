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
