//go:build windows

package acpclient

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func backgroundAuditLockContended(auditPath string) (bool, error) {
	parentPath := filepath.Dir(auditPath)
	parent, _, err := backgroundOpenWindowsAuditParent(parentPath)
	if err != nil {
		return false, err
	}
	lock, _, err := backgroundOpenWindowsAuditFile(filepath.Join(parentPath, backgroundAuditLockName), false)
	if err != nil {
		return false, errors.Join(err, parent.Close())
	}
	overlapped := new(windows.Overlapped)
	err = windows.LockFileEx(
		windows.Handle(lock.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, errors.Join(lock.Close(), parent.Close())
	}
	if err != nil {
		return false, errors.Join(err, lock.Close(), parent.Close())
	}
	return false, errors.Join(
		errors.New("expected audit lock contention"),
		backgroundReleaseWindowsAuditLock(lock, overlapped),
		parent.Close(),
	)
}
