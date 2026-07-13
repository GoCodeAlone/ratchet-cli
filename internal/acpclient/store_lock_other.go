//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package acpclient

import (
	"os"
	"path/filepath"
)

func acquireStoreFileLock(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f.Close, nil
}
