//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package hooks

import "errors"

var errManagedPolicyUnsupportedPlatform = errors.New("managed hook policy is unsupported on this platform")

func defaultManagedPolicyPath() (string, error) {
	return "", errManagedPolicyUnsupportedPlatform
}

func secureReadManagedFile(string) ([]byte, error) {
	return nil, errManagedPolicyUnsupportedPlatform
}
