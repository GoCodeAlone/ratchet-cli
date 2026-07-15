//go:build !darwin && !linux && !windows

package hooks

// Remaining Unix targets enforce the portable POSIX ACL contract through the
// effective group-class mask already exposed in os.FileMode.
func validatePlatformMutationACL(string) error { return nil }

func validateOpenedPlatformMutationACL(string, int) error { return nil }
