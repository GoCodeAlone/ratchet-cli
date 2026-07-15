//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package hooks

import (
	"errors"
	"testing"
)

func TestManagedUnsupportedPlatformFailsClosed(t *testing.T) {
	path, err := defaultManagedPolicyPath()
	if path != "" || !errors.Is(err, errManagedPolicyUnsupportedPlatform) {
		t.Fatalf("default path = %q, %v", path, err)
	}
	data, err := secureReadManagedFile("/managed/hooks.yaml")
	if data != nil || !errors.Is(err, errManagedPolicyUnsupportedPlatform) {
		t.Fatalf("secure read = %q, %v", data, err)
	}
	t.Run("explicit path", func(t *testing.T) {
		_, err := LoadManagedPolicy(LoadOptions{ManagedPath: "/managed/hooks.yaml"})
		if !errors.Is(err, ErrManagedPolicy) || !errors.Is(err, errManagedPolicyUnsupportedPlatform) {
			t.Fatalf("LoadManagedPolicy error = %v, want typed unsupported error", err)
		}
	})
	t.Run("default path", func(t *testing.T) {
		_, err := LoadManagedPolicy(LoadOptions{})
		if !errors.Is(err, ErrManagedPolicy) || !errors.Is(err, errManagedPolicyUnsupportedPlatform) {
			t.Fatalf("LoadManagedPolicy default error = %v, want typed unsupported error", err)
		}
	})
}
