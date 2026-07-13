//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package acpclient

import (
	"errors"
	"testing"
)

func TestStoreProcessLocksFailExplicitlyOnUnsupportedPlatforms(t *testing.T) {
	if release, err := acquireStoreFileLock("sessions.json.lock"); release != nil || !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("acquireStoreFileLock = %p, %v; want nil, ErrStoreProcessLockUnsupported", release, err)
	}
	if release, acquired, err := tryStoreFileLock("owner.lock"); release != nil || acquired || !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("tryStoreFileLock = %p, %t, %v; want nil, false, ErrStoreProcessLockUnsupported", release, acquired, err)
	}
}
