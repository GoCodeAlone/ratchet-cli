//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func backgroundAcquireUnixAuditLock(parent *os.File, parentPath string) (*os.File, error) {
	lockPath := filepath.Join(parentPath, backgroundAuditLockName)
	lock, err := backgroundOpenUnixRegularAt(parent, lockPath, unix.O_RDWR, true)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		_ = lock.Close()
		return nil, err
	}
	if err := backgroundValidateUnixRegularAtIdentity(parent, lockPath, lock); err != nil {
		return nil, errors.Join(err, backgroundReleaseUnixAuditLock(lock))
	}
	return lock, nil
}

func backgroundReleaseUnixAuditLock(lock *os.File) error {
	if lock == nil {
		return nil
	}
	return errors.Join(unix.Flock(int(lock.Fd()), unix.LOCK_UN), lock.Close())
}
