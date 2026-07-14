//go:build windows

package hooks

import (
	"errors"

	"golang.org/x/sys/windows"
)

const hookAuditProcessLockSuffix = ".lock"

func acquireHookAuditProcessLock(path string) (func() error, error) {
	lockPath := path + hookAuditProcessLockSuffix
	file, _, err := openHookAuditFile(lockPath, true)
	if err != nil {
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err := validateHookAuditIdentity(lockPath, file); err != nil {
		return nil, errors.Join(err, windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped), file.Close())
	}
	return func() error {
		return errors.Join(windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped), file.Close())
	}, nil
}
