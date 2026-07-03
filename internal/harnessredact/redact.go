package harnessredact

import (
	"sort"
	"strings"
)

// Redactor replaces sensitive harness strings before failure output is logged.
type Redactor struct {
	replacements []replacement
}

type replacement struct {
	raw    string
	marker string
}

// New builds a redactor for common smoke-harness sensitive strings.
func New(values ...string) Redactor {
	markers := []string{
		"<home>",
		"<workspace>",
		"<temp>",
		"<socket>",
		"<executable>",
		"<artifact>",
		"<prompt>",
		"<trust-body>",
	}
	repls := make([]replacement, 0, len(values))
	for i, raw := range values {
		if raw == "" {
			continue
		}
		marker := "<redacted>"
		if i < len(markers) {
			marker = markers[i]
		}
		repls = append(repls, replacement{raw: raw, marker: marker})
	}
	sort.SliceStable(repls, func(i, j int) bool {
		return len(repls[i].raw) > len(repls[j].raw)
	})
	return Redactor{replacements: repls}
}

// String redacts all configured values from s.
func (r Redactor) String(s string) string {
	out := s
	for _, repl := range r.replacements {
		out = strings.ReplaceAll(out, repl.raw, repl.marker)
	}
	return out
}
