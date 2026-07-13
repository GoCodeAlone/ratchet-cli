//go:build unix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package acpclient

import "os"

func backgroundAcquireUnixAuditLock(*os.File, string) (*os.File, error) {
	return nil, ErrStoreProcessLockUnsupported
}

func backgroundReleaseUnixAuditLock(lock *os.File) error {
	if lock == nil {
		return nil
	}
	return lock.Close()
}
