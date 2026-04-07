package mesh

import (
	"strings"
	"sync"
)

// ProjectBlackboard manages per-team Blackboard instances with configurable
// visibility modes (shared, isolated, orchestrator, bridge).
type ProjectBlackboard struct {
	mu    sync.RWMutex
	root  *Blackboard            // project-level shared BB
	teams map[string]*Blackboard // team name → team BB
	modes map[string]string      // team name → mode
}

// NewProjectBlackboard creates a project-level BB coordinator.
func NewProjectBlackboard() *ProjectBlackboard {
	return &ProjectBlackboard{
		root:  NewBlackboard(),
		teams: make(map[string]*Blackboard),
		modes: make(map[string]string),
	}
}

// Root returns the project-level shared Blackboard.
func (pbb *ProjectBlackboard) Root() *Blackboard {
	return pbb.root
}

// TeamBB returns a Blackboard for the team based on its configured mode.
//
// Modes:
//   - "shared" (default): Teams write to the root BB; full cross-team visibility.
//   - "isolated": Private BB with no cross-team reads or writes.
//   - "orchestrator": Private BB; orchestrator also has read access to root.
//   - "bridge:<t1>,<t2>": Teams named in the bridge share one BB instance.
func (pbb *ProjectBlackboard) TeamBB(teamName, mode string) *Blackboard {
	pbb.mu.Lock()
	defer pbb.mu.Unlock()

	if mode == "" {
		mode = "shared"
	}
	pbb.modes[teamName] = mode

	switch {
	case mode == "shared":
		// Shared: all teams write to the root BB under their own section names.
		pbb.teams[teamName] = pbb.root
		return pbb.root

	case mode == "isolated":
		if bb, ok := pbb.teams[teamName]; ok {
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[teamName] = bb
		return bb

	case mode == "orchestrator":
		if bb, ok := pbb.teams[teamName]; ok {
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[teamName] = bb
		return bb

	case strings.HasPrefix(mode, "bridge:"):
		// Bridge teams share a single BB instance keyed by the full mode string.
		bridgeKey := mode
		if bb, ok := pbb.teams[bridgeKey]; ok {
			pbb.teams[teamName] = bb
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[bridgeKey] = bb
		pbb.teams[teamName] = bb
		return bb

	default:
		return pbb.root
	}
}

// Mode returns the configured BB mode for a team.
func (pbb *ProjectBlackboard) Mode(teamName string) string {
	pbb.mu.RLock()
	defer pbb.mu.RUnlock()
	if m, ok := pbb.modes[teamName]; ok {
		return m
	}
	return "shared"
}
