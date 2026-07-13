//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	if err := unix.Flock(int(f.Fd()), flags); err != nil {
		_ = f.Close()
		if nonblocking && (errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(f.Fd()), unix.LOCK_UN), f.Close())
	}, true, nil
}
