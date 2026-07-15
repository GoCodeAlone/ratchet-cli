package hooks

import (
	"errors"
	"strings"
)

func validateHookAuditLinuxACLNames(names []string) error {
	for _, name := range names {
		lower := strings.ToLower(name)
		switch lower {
		case "system.posix_acl_access", "system.posix_acl_default":
			continue
		}
		if strings.Contains(lower, "acl") &&
			(strings.HasPrefix(lower, "system.") || strings.HasPrefix(lower, "security.") || strings.HasPrefix(lower, "trusted.")) {
			return errors.New("managed hook audit trusted anchor uses an unsupported Linux ACL model")
		}
	}
	return nil
}
