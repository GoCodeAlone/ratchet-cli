package plugins

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InstallFromGitHub downloads the latest release tarball from a GitHub repo
// and installs it to ~/.ratchet/plugins/<name>/.
// repo must be in "owner/name" format.
func InstallFromGitHub(ctx context.Context, repo string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repo format %q: expected owner/name", repo)
	}
	name := parts[1]

	tmpDir, err := os.MkdirTemp("", "ratchet-plugin-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.CommandContext(ctx, "gh", "release", "download",
		"--repo", repo,
		"--pattern", "*.tar.gz",
		"--dir", tmpDir,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh release download: %w", err)
	}

	// Find the downloaded tarball
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read temp dir: %w", err)
	}
	var tarball string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			tarball = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if tarball == "" {
		return fmt.Errorf("no .tar.gz found in release download")
	}

	destDir := filepath.Join(pluginsDir(), name)
	if err := extractTarGz(tarball, destDir); err != nil {
		os.RemoveAll(destDir)
		return fmt.Errorf("extract tarball: %w", err)
	}

	m, err := LoadManifest(destDir)
	if err != nil {
		os.RemoveAll(destDir)
		return fmt.Errorf("verify manifest: %w", err)
	}

	reg, err := Load()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	return reg.Add(name, RegistryEntry{
		Source:      "github:" + repo,
		Version:     m.Version,
		InstalledAt: time.Now(),
		Path:        destDir,
	})
}

// InstallFromLocal copies src to ~/.ratchet/plugins/<name>/ and registers it.
// src must contain a valid plugin manifest. Pass symlink=true to symlink instead of copy (for dev).
func InstallFromLocal(src string) error {
	m, err := LoadManifest(src)
	if err != nil {
		return fmt.Errorf("verify manifest at %s: %w", src, err)
	}

	destDir := filepath.Join(pluginsDir(), m.Name)
	// Remove any existing install to prevent stale files from a previous version.
	_ = os.RemoveAll(destDir)
	if err := copyDir(src, destDir); err != nil {
		os.RemoveAll(destDir) // clean up partial copy
		return fmt.Errorf("copy plugin: %w", err)
	}

	reg, err := Load()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		abs = src
	}
	return reg.Add(m.Name, RegistryEntry{
		Source:      "local:" + abs,
		Version:     m.Version,
		InstalledAt: time.Now(),
		Path:        destDir,
	})
}

// Uninstall removes a plugin directory and its registry entry.
func Uninstall(name string) error {
	dir := filepath.Join(pluginsDir(), name)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plugin dir: %w", err)
	}
	reg, err := Load()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	return reg.Remove(name)
}

// copyDir recursively copies src directory to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractTarGz extracts a .tar.gz archive to destDir.
func extractTarGz(tarball, destDir string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Strip leading path component (common in GitHub release tarballs)
		parts := strings.SplitN(filepath.ToSlash(hdr.Name), "/", 2)
		var rel string
		if len(parts) == 2 {
			rel = parts[1]
		} else {
			rel = hdr.Name
		}
		if rel == "" {
			continue
		}

		target := filepath.Join(destDir, filepath.FromSlash(rel))
		// Prevent path traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
