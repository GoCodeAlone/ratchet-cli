package acpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const storeLockDirectoryName = ".ratchet-locks"

var ErrStoreLockPathUnsafe = errors.New("acp client store lock path is unsafe")

func storeLockPhysicalPath(logicalPath string) (string, error) {
	canonicalPath, err := filepath.Abs(filepath.Clean(logicalPath))
	if err != nil {
		return "", err
	}
	canonicalDir := storeLockCanonicalDir(filepath.Dir(canonicalPath))
	canonicalPath = filepath.Join(canonicalDir, filepath.Base(canonicalPath))
	if storeLockFoldsCase(runtime.GOOS) {
		canonicalPath = strings.ToLower(canonicalPath)
	}
	digest := sha256.Sum256([]byte(canonicalPath))
	return filepath.Join(canonicalDir, storeLockDirectoryName, hex.EncodeToString(digest[:])+".lock"), nil
}

func storeLockFoldsCase(goos string) bool {
	return goos == "darwin" || goos == "windows"
}

func storeLockUnsafePathError(path string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrStoreLockPathUnsafe, path, err)
}

func storeLockCanonicalDir(path string) string {
	current := path
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
