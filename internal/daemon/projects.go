package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Project is a registered multi-team project.
type Project struct {
	ID         string
	Name       string
	ConfigPath string
	Status     string // active, paused, killed, completed
	TeamIDs    []string
	CreatedAt  time.Time
}

// ProjectRegistry manages active projects in memory.
type ProjectRegistry struct {
	mu       sync.RWMutex
	projects map[string]*Project // keyed by ID
	byName   map[string]string   // name → ID
}

// NewProjectRegistry returns an initialized ProjectRegistry.
func NewProjectRegistry() *ProjectRegistry {
	return &ProjectRegistry{
		projects: make(map[string]*Project),
		byName:   make(map[string]string),
	}
}

// Register creates a new project entry.
func (pr *ProjectRegistry) Register(name, configPath string) (*Project, error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.byName[name]; exists {
		return nil, fmt.Errorf("project %q already exists", name)
	}

	p := &Project{
		ID:         "proj-" + uuid.NewString()[:8],
		Name:       name,
		ConfigPath: configPath,
		Status:     "active",
		CreatedAt:  time.Now(),
	}
	pr.projects[p.ID] = p
	pr.byName[name] = p.ID
	return p, nil
}

// Get returns a project by ID.
func (pr *ProjectRegistry) Get(id string) (*Project, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	p, ok := pr.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	cp := *p
	cp.TeamIDs = make([]string, len(p.TeamIDs))
	copy(cp.TeamIDs, p.TeamIDs)
	return &cp, nil
}

// GetByName returns a project by name.
func (pr *ProjectRegistry) GetByName(name string) (*Project, error) {
	pr.mu.RLock()
	id, ok := pr.byName[name]
	pr.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return pr.Get(id)
}

// List returns all projects.
func (pr *ProjectRegistry) List() []Project {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	out := make([]Project, 0, len(pr.projects))
	for _, p := range pr.projects {
		cp := *p
		cp.TeamIDs = make([]string, len(p.TeamIDs))
		copy(cp.TeamIDs, p.TeamIDs)
		out = append(out, cp)
	}
	return out
}

// SetStatus updates the project status.
func (pr *ProjectRegistry) SetStatus(id, status string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	p, ok := pr.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	p.Status = status
	return nil
}

// AddTeam associates a team ID with the project.
func (pr *ProjectRegistry) AddTeam(projectID, teamID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if p, ok := pr.projects[projectID]; ok {
		p.TeamIDs = append(p.TeamIDs, teamID)
	}
}

// RemoveTeam disassociates a team from the project.
func (pr *ProjectRegistry) RemoveTeam(projectID, teamID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	p, ok := pr.projects[projectID]
	if !ok {
		return
	}
	for i, id := range p.TeamIDs {
		if id == teamID {
			p.TeamIDs = append(p.TeamIDs[:i], p.TeamIDs[i+1:]...)
			return
		}
	}
}
