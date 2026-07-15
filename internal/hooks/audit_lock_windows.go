//go:build windows

package hooks

import (
	"errors"

	"golang.org/x/sys/windows"
)

const hookAuditProcessLockSuffix = ".lock"

func acquireHookAuditProcessLock(path string, before, after, beforeUnlock func() error) (func() error, error) {
	lockPath := path + hookAuditProcessLockSuffix
	file, _, err := openHookAuditFile(lockPath, true)
	if err != nil {
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if before != nil {
		if err := before(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if after != nil {
		if err := after(); err != nil {
			return nil, errors.Join(err, windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped), file.Close())
		}
	}
	if err := validateHookAuditIdentity(lockPath, file); err != nil {
		return nil, errors.Join(err, windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped), file.Close())
	}
	return func() error {
		var beforeUnlockErr error
		if beforeUnlock != nil {
			beforeUnlockErr = beforeUnlock()
		}
		return errors.Join(beforeUnlockErr, windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped), file.Close())
	}, nil
}
