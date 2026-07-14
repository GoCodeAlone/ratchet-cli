//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package hooks

import "errors"

func acquireHookAuditProcessLock(string, func() error, func() error) (func() error, error) {
	return nil, errors.New("managed hook audit process lock is unsupported on this platform")
}
