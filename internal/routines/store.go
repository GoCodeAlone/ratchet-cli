package routines

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

	"github.com/GoCodeAlone/ratchet-cli/internal/storefile"
)

const RunStatusRecorded = "recorded"

type Definition struct {
	ID        string       `json:"id"`
	Schedule  string       `json:"schedule"`
	Prompt    string       `json:"prompt"`
	CWD       string       `json:"cwd,omitempty"`
	Provider  string       `json:"provider,omitempty"`
	Paused    bool         `json:"paused"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	LastRun   *RunMetadata `json:"lastRun,omitempty"`
}

type RunMetadata struct {
	RunID  string    `json:"runId"`
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

type Run struct {
	ID        string    `json:"id"`
	RoutineID string    `json:"routineId"`
	Status    string    `json:"status"`
	Prompt    string    `json:"prompt"`
	CWD       string    `json:"cwd,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type AddRequest struct {
	Schedule string
	Prompt   string
	CWD      string
	Provider string
}

type Store struct {
	Definitions map[string]Definition `json:"definitions"`
	Runs        map[string]Run        `json:"runs"`
	filePath    string
	now         func() time.Time
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "routines", "routines.json")
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
		return nil, fmt.Errorf("read routines store: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse routines store: %w", err)
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

func (s *Store) Add(req AddRequest) (Definition, error) {
	req.Schedule = strings.TrimSpace(req.Schedule)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.CWD = strings.TrimSpace(req.CWD)
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Schedule == "" {
		return Definition{}, fmt.Errorf("routine schedule is required")
	}
	if req.Prompt == "" {
		return Definition{}, fmt.Errorf("routine prompt is required")
	}
	now := s.now().UTC()
	def := Definition{
		ID:        uuid.NewString(),
		Schedule:  req.Schedule,
		Prompt:    req.Prompt,
		CWD:       req.CWD,
		Provider:  req.Provider,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.Definitions[def.ID] = def
	return def, s.Save()
}

func (s *Store) List() []Definition {
	out := make([]Definition, 0, len(s.Definitions))
	for _, def := range s.Definitions {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Store) Get(id string) (Definition, bool) {
	def, ok := s.Definitions[strings.TrimSpace(id)]
	return def, ok
}

func (s *Store) Pause(id string) error {
	return s.setPaused(id, true)
}

func (s *Store) Resume(id string) error {
	return s.setPaused(id, false)
}

func (s *Store) setPaused(id string, paused bool) error {
	id = strings.TrimSpace(id)
	def, ok := s.Definitions[id]
	if !ok {
		return fmt.Errorf("routine %q not found", id)
	}
	def.Paused = paused
	def.UpdatedAt = s.now().UTC()
	s.Definitions[id] = def
	return s.Save()
}

func (s *Store) Remove(id string) error {
	id = strings.TrimSpace(id)
	if _, ok := s.Definitions[id]; !ok {
		return fmt.Errorf("routine %q not found", id)
	}
	delete(s.Definitions, id)
	return s.Save()
}

func (s *Store) RunManual(id string) (Run, error) {
	id = strings.TrimSpace(id)
	def, ok := s.Definitions[id]
	if !ok {
		return Run{}, fmt.Errorf("routine %q not found", id)
	}
	now := s.now().UTC()
	run := Run{
		ID:        uuid.NewString(),
		RoutineID: def.ID,
		Status:    RunStatusRecorded,
		Prompt:    def.Prompt,
		CWD:       def.CWD,
		Provider:  def.Provider,
		CreatedAt: now,
	}
	def.LastRun = &RunMetadata{RunID: run.ID, Status: run.Status, At: now}
	def.UpdatedAt = now
	s.Definitions[def.ID] = def
	s.Runs[run.ID] = run
	return run, s.Save()
}

func (s *Store) RunsForRoutine(id string) []Run {
	out := make([]Run, 0)
	for _, run := range s.Runs {
		if run.RoutineID == id {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Store) Save() error {
	if err := storefile.WriteJSON(s.filePath, s, 0o600); err != nil {
		return fmt.Errorf("save routines store: %w", err)
	}
	return nil
}
