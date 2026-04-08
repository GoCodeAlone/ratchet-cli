package config

import (
	"strings"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

// TrustConfig is the trust section of ~/.ratchet/config.yaml.
type TrustConfig struct {
	Mode         string              `yaml:"mode"`
	Rules        []TrustRuleConfig   `yaml:"rules"`
	ProviderArgs map[string][]string `yaml:"provider_args"`
}

// TrustRuleConfig is a single trust rule in the ratchet config format.
type TrustRuleConfig struct {
	Pattern string `yaml:"pattern"`
	Action  string `yaml:"action"`
}

// ToTrustRules converts the config rules into policy.TrustRule values.
func (tc *TrustConfig) ToTrustRules() []policy.TrustRule {
	var rules []policy.TrustRule
	for _, r := range tc.Rules {
		var action policy.Action
		switch strings.ToLower(r.Action) {
		case "allow":
			action = policy.Allow
		case "deny":
			action = policy.Deny
		case "ask":
			action = policy.Ask
		default:
			action = policy.Deny
		}
		rules = append(rules, policy.TrustRule{
			Pattern: r.Pattern,
			Action:  action,
			Scope:   "global",
		})
	}
	return rules
}

// ProviderArgsFor returns CLI args for the given provider name.
func (tc *TrustConfig) ProviderArgsFor(providerName string) []string {
	if tc.ProviderArgs == nil {
		return nil
	}
	return tc.ProviderArgs[providerName]
}
