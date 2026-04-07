package mesh

import (
	"fmt"
	"time"
)

// WriteHandoff writes a handoff payload from one team to another.
// Stored in BB section "handoffs/<from>-to-<to>".
func WriteHandoff(bb *Blackboard, fromTeam, toTeam string, data map[string]string) {
	section := fmt.Sprintf("handoffs/%s-to-%s", fromTeam, toTeam)
	for k, v := range data {
		bb.Write(section, k, v, fromTeam)
	}
	bb.Write(section, "_timestamp", time.Now().Format(time.RFC3339), fromTeam)
}

// ReadHandoff reads the handoff payload from one team to another.
// Returns the data map and true if found, nil and false otherwise.
func ReadHandoff(bb *Blackboard, fromTeam, toTeam string) (map[string]string, bool) {
	section := fmt.Sprintf("handoffs/%s-to-%s", fromTeam, toTeam)
	entries := bb.List(section)
	if len(entries) == 0 {
		return nil, false
	}
	result := make(map[string]string, len(entries))
	for k, e := range entries {
		if k == "_timestamp" || k == "_init" {
			continue
		}
		result[k] = fmt.Sprintf("%v", e.Value)
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

// WriteDirective writes a directive from oversight to a team.
// Stored in BB section "directives/<toTeam>".
func WriteDirective(bb *Blackboard, toTeam, directive string) {
	section := fmt.Sprintf("directives/%s", toTeam)
	key := fmt.Sprintf("d-%d", time.Now().UnixMilli())
	bb.Write(section, key, directive, "oversight")
}

// ReadLatestDirective reads the most recent directive for a team.
func ReadLatestDirective(bb *Blackboard, teamName string) (string, bool) {
	section := fmt.Sprintf("directives/%s", teamName)
	entries := bb.List(section)
	if len(entries) == 0 {
		return "", false
	}
	var latest Entry
	for k, e := range entries {
		if k == "_init" {
			continue
		}
		if e.Revision > latest.Revision {
			latest = e
		}
	}
	if latest.Key == "" {
		return "", false
	}
	return fmt.Sprintf("%v", latest.Value), true
}
