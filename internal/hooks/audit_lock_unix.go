//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package hooks

import (
	"errors"

	"golang.org/x/sys/unix"
)

const hookAuditProcessLockSuffix = ".lock"

func acquireHookAuditProcessLock(path string, before, after func() error) (func() error, error) {
	lockPath := path + hookAuditProcessLockSuffix
	file, _, err := openHookAuditFile(lockPath, true)
	if err != nil {
		return nil, err
	}
	if before != nil {
		if err := before(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if after != nil {
		if err := after(); err != nil {
			return nil, errors.Join(err, unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
		}
	}
	if err := validateHookAuditIdentity(lockPath, file); err != nil {
		return nil, errors.Join(err, unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
	}
	return func() error {
		return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
	}, nil
}
