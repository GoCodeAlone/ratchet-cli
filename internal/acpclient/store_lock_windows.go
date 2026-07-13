//go:build windows

package acpclient

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func acquireStoreFileLock(path string) (func() error, error) {
	release, acquired, err := lockStoreFile(path, false)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("blocking store lock was not acquired")
	}
	return release, nil
}

func tryStoreFileLock(path string) (func() error, bool, error) {
	return lockStoreFile(path, true)
}

func lockStoreFile(path string, nonblocking bool) (func() error, bool, error) {
	if err := backgroundEnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := backgroundSetPrivateACL(path); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	var overlapped windows.Overlapped
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &overlapped); err != nil {
		_ = f.Close()
		if nonblocking && errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() error {
		return errors.Join(windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped), f.Close())
	}, true, nil
}
