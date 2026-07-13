//go:build windows

package acpclient

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func acquireStoreFileLock(path string) (func() error, error) {
	if err := backgroundEnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := backgroundSetPrivateACL(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		return errors.Join(windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped), f.Close())
	}, nil
}
