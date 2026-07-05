package releaseguard

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
)

type Mode string

const (
	ModeManifest     Mode = "manifest"
	ModeDraftAssets  Mode = "draft-assets"
	ModeTapPreflight Mode = "tap-preflight"
	ModeTapPostcheck Mode = "tap-postcheck"
)

var ErrArtifactModeNotRequested = errors.New("releaseguard artifact mode not requested")

var forbiddenTokens = []string{
	"ratchet-tui-smoke",
	"tui_smoke",
	"--tui-smoke",
	"ConnectSmokeUnix",
}

func RunFromEnv(root string) error {
	mode := Mode(os.Getenv("RATCHET_RELEASE_GUARD_MODE"))
	if mode == "" {
		return ErrArtifactModeNotRequested
	}
	var err error
	switch mode {
	case ModeManifest:
		dist := os.Getenv("RATCHET_RELEASE_GUARD_DIST")
		if dist == "" {
			return fmt.Errorf("RATCHET_RELEASE_GUARD_DIST is required for %s mode", mode)
		}
		dist = resolvePath(root, dist)
		err = GuardManifest(root, dist)
		return redactError(err, root, dist)
	case ModeDraftAssets:
		assets := os.Getenv("RATCHET_RELEASE_GUARD_ASSETS")
		version := os.Getenv("RATCHET_RELEASE_GUARD_VERSION")
		if assets == "" {
			return fmt.Errorf("RATCHET_RELEASE_GUARD_ASSETS is required for %s mode", mode)
		}
		if version == "" {
			return fmt.Errorf("RATCHET_RELEASE_GUARD_VERSION is required for %s mode", mode)
		}
		assets = resolvePath(root, assets)
		err = GuardDraftAssets(root, assets, version)
		return redactError(err, root, assets)
	case ModeTapPreflight:
		tap := os.Getenv("RATCHET_RELEASE_GUARD_TAP")
		if tap == "" {
			return fmt.Errorf("RATCHET_RELEASE_GUARD_TAP is required for %s mode", mode)
		}
		tap = resolvePath(root, tap)
		err = GuardTapPreflight(root, tap)
		return redactError(err, root, tap)
	case ModeTapPostcheck:
		tap := os.Getenv("RATCHET_RELEASE_GUARD_TAP")
		names := os.Getenv("RATCHET_RELEASE_GUARD_TAP_NAMES")
		commits := os.Getenv("RATCHET_RELEASE_GUARD_TAP_COMMITS")
		version := os.Getenv("RATCHET_RELEASE_GUARD_VERSION")
		for name, value := range map[string]string{
			"RATCHET_RELEASE_GUARD_TAP":         tap,
			"RATCHET_RELEASE_GUARD_TAP_NAMES":   names,
			"RATCHET_RELEASE_GUARD_TAP_COMMITS": commits,
			"RATCHET_RELEASE_GUARD_VERSION":     version,
		} {
			if value == "" {
				return fmt.Errorf("%s is required for %s mode", name, mode)
			}
		}
		tap = resolvePath(root, tap)
		err = GuardTapPostcheck(root, tap, names, commits, version)
		return redactError(err, root, tap)
	default:
		return fmt.Errorf("unknown RATCHET_RELEASE_GUARD_MODE %q", mode)
	}
}

func resolvePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func GuardManifest(root, dist string) error {
	cfg, err := LoadGoReleaserConfig(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		return err
	}
	if err := ValidateGoReleaserConfig(cfg); err != nil {
		return err
	}
	if err := ValidateHomebrewCaskConfig(cfg); err != nil {
		return err
	}
	sums, err := readChecksums(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		return err
	}
	for _, target := range archiveTargets() {
		name := target.archiveName()
		path := filepath.Join(dist, name)
		if err := verifyChecksum(path, name, sums); err != nil {
			return err
		}
		if err := scanArchive(path, target); err != nil {
			return err
		}
		if target.goos == runtime.GOOS && target.goarch == runtime.GOARCH {
			if err := executePackagedRatchet(path, target); err != nil {
				return err
			}
		}
	}
	return scanGeneratedMaterial(dist)
}

func GuardDraftAssets(root, assets, version string) error {
	if version == "" {
		return fmt.Errorf("draft asset version is required")
	}
	if err := GuardManifest(root, assets); err != nil {
		return err
	}
	metadataPath := filepath.Join(assets, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("read draft asset metadata: %w", err)
	}
	var metadata struct {
		Tag     string `json:"tag"`
		Version string `json:"version"`
		Draft   *bool  `json:"draft"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode draft asset metadata: %w", err)
	}
	if !versionMatches(metadata.Tag, version) && !versionMatches(metadata.Version, version) {
		return fmt.Errorf("draft asset metadata tag/version %q/%q does not match requested %q", metadata.Tag, metadata.Version, version)
	}
	if metadata.Draft == nil || !*metadata.Draft {
		return fmt.Errorf("draft asset metadata draft state must be true")
	}
	return nil
}

func versionMatches(got, want string) bool {
	return got == want || strings.TrimPrefix(got, "v") == strings.TrimPrefix(want, "v")
}

type archiveTarget struct {
	goos   string
	goarch string
}

func archiveTargets() []archiveTarget {
	targets := make([]archiveTarget, 0, 6)
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			targets = append(targets, archiveTarget{goos: goos, goarch: goarch})
		}
	}
	return targets
}

func (t archiveTarget) archiveName() string {
	if t.goos == "windows" {
		return fmt.Sprintf("ratchet_%s_%s.zip", t.goos, t.goarch)
	}
	return fmt.Sprintf("ratchet_%s_%s.tar.gz", t.goos, t.goarch)
}

func readChecksums(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	defer f.Close()
	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		out[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan checksums: %w", err)
	}
	return out, nil
}

func verifyChecksum(path, name string, sums map[string]string) error {
	want := sums[name]
	if want == "" {
		return fmt.Errorf("checksums.txt is missing %s", name)
	}
	got, err := sha256Hex(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func sha256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func scanArchive(path string, target archiveTarget) error {
	if token, ok := findForbiddenToken(path); ok {
		return fmt.Errorf("artifact name %s contains forbidden smoke token %q", path, token)
	}
	if target.goos == "windows" {
		return scanZipArchive(path)
	}
	return scanTarGzArchive(path)
}

func scanTarGzArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip %s: %w", path, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	executables := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if executables != 1 {
				return packagedBinaryCountError(path, executables)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar %s: %w", path, err)
		}
		if token, ok := findForbiddenToken(h.Name); ok {
			return fmt.Errorf("archive member %s contains forbidden smoke token %q", h.Name, token)
		}
		if isExecutableMember(h.Name) {
			executables++
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read tar member %s: %w", h.Name, err)
			}
			if token, ok := findForbiddenToken(string(data)); ok {
				return fmt.Errorf("archive member %s payload contains forbidden smoke token %q", h.Name, token)
			}
		}
	}
}

func scanZipArchive(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", path, err)
	}
	defer zr.Close()
	executables := 0
	for _, member := range zr.File {
		if token, ok := findForbiddenToken(member.Name); ok {
			return fmt.Errorf("archive member %s contains forbidden smoke token %q", member.Name, token)
		}
		if isExecutableMember(member.Name) {
			executables++
			rc, err := member.Open()
			if err != nil {
				return fmt.Errorf("open zip member %s: %w", member.Name, err)
			}
			data, readErr := io.ReadAll(rc)
			closeErr := rc.Close()
			if readErr != nil {
				return fmt.Errorf("read zip member %s: %w", member.Name, readErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close zip member %s: %w", member.Name, closeErr)
			}
			if token, ok := findForbiddenToken(string(data)); ok {
				return fmt.Errorf("archive member %s payload contains forbidden smoke token %q", member.Name, token)
			}
		}
	}
	if executables != 1 {
		return packagedBinaryCountError(path, executables)
	}
	return nil
}

func packagedBinaryCountError(path string, got int) error {
	if got == 0 {
		return fmt.Errorf("%s has 0 packaged ratchet binaries, want 1; missing packaged ratchet binary", path)
	}
	return fmt.Errorf("%s has %d packaged ratchet binaries, want 1", path, got)
}

func isExecutableMember(name string) bool {
	base := filepath.Base(name)
	return base == "ratchet" || base == "ratchet.exe"
}

func executePackagedRatchet(path string, target archiveTarget) error {
	tmp, err := os.MkdirTemp("", "ratchet-releaseguard-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	bin, err := extractRatchet(path, target, tmp)
	if err != nil {
		return err
	}
	for _, arg := range []string{"version", "help"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, bin, arg)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("packaged ratchet %s failed: %w\n%s", arg, err, out)
		}
		text := string(out)
		if !strings.Contains(strings.ToLower(text), "ratchet") {
			return fmt.Errorf("packaged ratchet %s output missing release identity text: %q", arg, text)
		}
		if token, ok := findForbiddenToken(text); ok {
			return fmt.Errorf("packaged ratchet %s output contains forbidden smoke token %q", arg, token)
		}
	}
	return nil
}

func extractRatchet(path string, target archiveTarget, dest string) (string, error) {
	want := "ratchet"
	if target.goos == "windows" {
		want = "ratchet.exe"
	}
	if target.goos == "windows" {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return "", err
		}
		defer zr.Close()
		for _, member := range zr.File {
			if filepath.Base(member.Name) != want {
				continue
			}
			return extractZipMember(member, filepath.Join(dest, want))
		}
		return "", fmt.Errorf("%s missing %s", path, want)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s missing %s", path, want)
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(h.Name) != want {
			continue
		}
		out := filepath.Join(dest, want)
		if err := writeExtracted(out, tr, fs.FileMode(h.Mode)); err != nil {
			return "", err
		}
		return out, nil
	}
}

func extractZipMember(member *zip.File, out string) (string, error) {
	rc, err := member.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	if err := writeExtracted(out, rc, 0o755); err != nil {
		return "", err
	}
	return out, nil
}

func writeExtracted(path string, r io.Reader, mode fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		closeErr := f.Close()
		return errors.Join(err, closeErr)
	}
	return f.Close()
}

func scanGeneratedMaterial(dist string) error {
	return filepath.WalkDir(dist, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.ToSlash(path)
		if !strings.HasSuffix(name, ".rb") && !strings.Contains(name, "homebrew") && !strings.Contains(name, "cask") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if token, ok := findForbiddenToken(string(data)); ok {
			return fmt.Errorf("generated material %s contains forbidden smoke token %q", path, token)
		}
		if err := validateGeneratedHomebrewMaterial(path, string(data)); err != nil {
			return err
		}
		return nil
	})
}

func validateGeneratedHomebrewMaterial(path, text string) error {
	if strings.Contains(filepath.ToSlash(path), "/Formula/") || strings.Contains(text, "class RatchetCli < Formula") {
		for _, want := range []string{`class RatchetCli < Formula`, `bin.install "ratchet"`} {
			if !strings.Contains(text, want) {
				return fmt.Errorf("generated formula material %s missing %s", path, want)
			}
		}
		return nil
	}
	for _, want := range []string{`cask "ratchet-cli"`, `binary "ratchet"`} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("generated cask material %s missing %s", path, want)
		}
	}
	return nil
}

func findForbiddenToken(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		for _, token := range forbiddenTokens {
			if strings.Contains(value, token) {
				return token, true
			}
		}
	case []any:
		for _, item := range value {
			if token, ok := findForbiddenToken(item); ok {
				return token, true
			}
		}
	case map[string]any:
		for key, item := range value {
			if token, ok := findForbiddenToken(key); ok {
				return token, true
			}
			if token, ok := findForbiddenToken(item); ok {
				return token, true
			}
		}
	case map[any]any:
		for key, item := range value {
			if token, ok := findForbiddenToken(fmt.Sprint(key)); ok {
				return token, true
			}
			if token, ok := findForbiddenToken(item); ok {
				return token, true
			}
		}
	case []string:
		for _, item := range value {
			if token, ok := findForbiddenToken(item); ok {
				return token, true
			}
		}
	}
	return "", false
}

func redactError(err error, root, artifact string) error {
	if err == nil {
		return nil
	}
	artifactParent := ""
	if artifact != "" {
		artifactParent = filepath.Dir(artifact)
	}
	redactor := harnessredact.New("", root, artifactParent, "", "", artifact)
	return errors.New(redactor.String(err.Error()))
}
