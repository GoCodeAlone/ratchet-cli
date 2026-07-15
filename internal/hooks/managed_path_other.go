//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package hooks

var errManagedPolicyUnsupportedPlatform = ErrManagedPolicyUnsupportedPlatform

func defaultManagedPolicyPath() (string, error) {
	return "", errManagedPolicyUnsupportedPlatform
}

func secureReadManagedFile(string) ([]byte, error) {
	return nil, errManagedPolicyUnsupportedPlatform
}
