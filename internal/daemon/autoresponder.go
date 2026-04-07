package daemon

import (
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AutorespondRule defines a pattern-matching rule.
type AutorespondRule struct {
	Match   string `yaml:"match"`
	Action  string `yaml:"action"`  // "approve", "reply", "queue"
	Message string `yaml:"message"` // used when action is "reply"
}

// AutorespondConfig is the top-level config from .ratchet/autorespond.yaml.
type AutorespondConfig struct {
	Rules []AutorespondRule `yaml:"rules"`
}

// Autoresponder evaluates incoming human requests against rules.
type Autoresponder struct {
	rules []autoresponderCompiledRule
}

type autoresponderCompiledRule struct {
	pattern  *regexp.Regexp
	action   string
	message  string
	catchAll bool
}

// NewAutoresponder compiles rules into an evaluator.
func NewAutoresponder(rules []AutorespondRule) *Autoresponder {
	compiled := make([]autoresponderCompiledRule, 0, len(rules))
	for _, r := range rules {
		cr := autoresponderCompiledRule{
			action:  r.Action,
			message: r.Message,
		}
		if r.Match == "*" {
			cr.catchAll = true
		} else {
			re, err := regexp.Compile("(?i)" + r.Match)
			if err != nil {
				continue
			}
			cr.pattern = re
		}
		compiled = append(compiled, cr)
	}
	return &Autoresponder{rules: compiled}
}

// LoadAutoresponder reads .ratchet/autorespond.yaml if it exists.
// Returns nil if the file does not exist or cannot be parsed.
func LoadAutoresponder(projectDir string) *Autoresponder {
	path := projectDir + "/.ratchet/autorespond.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg AutorespondConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return NewAutoresponder(cfg.Rules)
}

// Match evaluates the question against rules and returns (action, message).
// Returns ("queue", "") if no rule matches.
func (ar *Autoresponder) Match(question string) (string, string) {
	lower := strings.ToLower(question)
	for _, r := range ar.rules {
		if r.catchAll {
			return r.action, r.message
		}
		if r.pattern != nil && r.pattern.MatchString(lower) {
			switch r.action {
			case "approve":
				return "approve", "approved"
			case "reply":
				return "reply", r.message
			default:
				return r.action, r.message
			}
		}
	}
	return "queue", ""
}
