//go:build !darwin && !windows

package hooks

func validateHookAuditPlatformACL(string) error { return nil }
