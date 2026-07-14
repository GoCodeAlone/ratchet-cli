//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func backgroundAuditLockContended(auditPath string) (bool, error) {
	parentPath := filepath.Dir(auditPath)
	parent, err := backgroundOpenUnixPrivateDir(parentPath)
	if err != nil {
		return false, err
	}
	lockPath := filepath.Join(parentPath, backgroundAuditLockName)
	lock, err := backgroundOpenUnixRegularAt(parent, lockPath, unix.O_RDWR, false)
	if err != nil {
		return false, errors.Join(err, parent.Close())
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return true, errors.Join(lock.Close(), parent.Close())
		}
		return false, errors.Join(err, lock.Close(), parent.Close())
	}
	return false, errors.Join(
		errors.New("expected audit lock contention"),
		backgroundReleaseUnixAuditLock(lock),
		parent.Close(),
	)
}
