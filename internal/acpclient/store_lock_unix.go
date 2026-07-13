//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
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
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(f.Fd()), unix.LOCK_UN), f.Close())
	}, nil
}
