package workflows

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/GoCodeAlone/ratchet-cli/internal/storefile"
)

const (
	RunStatusRunning = "running"
	RunStatusStopped = "stopped"
)

type Definition struct {
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Nodes       []Node    `json:"nodes" yaml:"nodes"`
	Edges       []Edge    `json:"edges,omitempty" yaml:"edges,omitempty"`
	Source      string    `json:"source,omitempty" yaml:"-"`
	CreatedAt   time.Time `json:"createdAt" yaml:"-"`
	UpdatedAt   time.Time `json:"updatedAt" yaml:"-"`
}

type Node struct {
	ID     string         `json:"id" yaml:"id"`
	Type   string         `json:"type" yaml:"type"`
	Prompt string         `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Inputs map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

type Edge struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

type Run struct {
	ID           string    `json:"id"`
	WorkflowName string    `json:"workflowName"`
	Status       string    `json:"status"`
	ParentRunID  string    `json:"parentRunId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Store struct {
	Definitions map[string]Definition `json:"definitions"`
	Runs        map[string]Run        `json:"runs"`
	filePath    string
	now         func() time.Time
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "workflows", "workflows.json")
}

func LoadDefault() (*Store, error) {
	return Load(DefaultPath())
}

func Load(path string) (*Store, error) {
	s := &Store{
		Definitions: make(map[string]Definition),
		Runs:        make(map[string]Run),
		filePath:    path,
		now:         time.Now,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read workflows store: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse workflows store: %w", err)
	}
	s.filePath = path
	s.now = time.Now
	if s.Definitions == nil {
		s.Definitions = make(map[string]Definition)
	}
	if s.Runs == nil {
		s.Runs = make(map[string]Run)
	}
	return s, nil
}

func (s *Store) InstallFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read workflow definition: %w", err)
	}
	var def Definition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return Definition{}, fmt.Errorf("parse workflow definition: %w", err)
	}
	def.Source = path
	return s.Install(def)
}

func (s *Store) Install(def Definition) (Definition, error) {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	if def.Name == "" {
		return Definition{}, fmt.Errorf("workflow name is required")
	}
	if strings.ContainsAny(def.Name, "/|") {
		return Definition{}, fmt.Errorf("workflow name %q cannot contain '/' or '|'", def.Name)
	}
	if err := validateGraph(def); err != nil {
		return Definition{}, err
	}
	now := s.now().UTC()
	if existing, ok := s.Definitions[def.Name]; ok {
		def.CreatedAt = existing.CreatedAt
	} else {
		def.CreatedAt = now
	}
	def.UpdatedAt = now
	s.Definitions[def.Name] = def
	return def, s.Save()
}

func (s *Store) List() []Definition {
	out := make([]Definition, 0, len(s.Definitions))
	for _, def := range s.Definitions {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Get(name string) (Definition, bool) {
	def, ok := s.Definitions[strings.TrimSpace(name)]
	return def, ok
}

func (s *Store) Run(name string) (Run, error) {
	name = strings.TrimSpace(name)
	if _, ok := s.Definitions[name]; !ok {
		return Run{}, fmt.Errorf("workflow %q not found", name)
	}
	now := s.now().UTC()
	run := Run{
		ID:           uuid.NewString(),
		WorkflowName: name,
		Status:       RunStatusRunning,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.Runs[run.ID] = run
	return run, s.Save()
}

func (s *Store) GetRun(id string) (Run, bool) {
	run, ok := s.Runs[strings.TrimSpace(id)]
	return run, ok
}

func (s *Store) Stop(id string) error {
	id = strings.TrimSpace(id)
	run, ok := s.Runs[id]
	if !ok {
		return fmt.Errorf("workflow run %q not found", id)
	}
	if run.Status == RunStatusStopped {
		return nil
	}
	if run.Status != RunStatusRunning {
		return fmt.Errorf("workflow run %q cannot transition from %s to %s", id, run.Status, RunStatusStopped)
	}
	run.Status = RunStatusStopped
	run.UpdatedAt = s.now().UTC()
	s.Runs[id] = run
	return s.Save()
}

func (s *Store) Resume(id string) (Run, error) {
	id = strings.TrimSpace(id)
	run, ok := s.Runs[id]
	if !ok {
		return Run{}, fmt.Errorf("workflow run %q not found", id)
	}
	if run.Status != RunStatusStopped {
		return Run{}, fmt.Errorf("workflow run %q is not stopped", id)
	}
	now := s.now().UTC()
	resumed := Run{
		ID:           uuid.NewString(),
		WorkflowName: run.WorkflowName,
		Status:       RunStatusRunning,
		ParentRunID:  run.ID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.Runs[resumed.ID] = resumed
	return resumed, s.Save()
}

func (s *Store) Save() error {
	if err := storefile.WriteJSON(s.filePath, s, 0o600); err != nil {
		return fmt.Errorf("save workflows store: %w", err)
	}
	return nil
}

func validateGraph(def Definition) error {
	if len(def.Nodes) == 0 {
		return fmt.Errorf("workflow %q must define at least one node", def.Name)
	}
	seen := make(map[string]struct{}, len(def.Nodes))
	for _, node := range def.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			return fmt.Errorf("workflow %q has a node without id", def.Name)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("workflow %q has duplicate node %q", def.Name, id)
		}
		seen[id] = struct{}{}
		switch strings.ToLower(strings.TrimSpace(node.Type)) {
		case "shell", "command", "javascript", "js":
			return fmt.Errorf("workflow %q node %q uses deferred executable type %q", def.Name, node.ID, node.Type)
		case "":
			return fmt.Errorf("workflow %q node %q has no type", def.Name, node.ID)
		}
	}
	for _, edge := range def.Edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if _, ok := seen[from]; !ok {
			return fmt.Errorf("workflow %q edge references missing from node %q", def.Name, edge.From)
		}
		if _, ok := seen[to]; !ok {
			return fmt.Errorf("workflow %q edge references missing to node %q", def.Name, edge.To)
		}
	}
	return nil
}
