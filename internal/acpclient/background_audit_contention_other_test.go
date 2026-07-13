//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package acpclient

func backgroundAuditLockContended(string) (bool, error) {
	return false, ErrStoreProcessLockUnsupported
}
