package acpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CompareRunStore struct {
	root string
}

type CompareRun struct {
	RunID        string       `json:"run_id"`
	RunDir       string       `json:"run_dir"`
	Status       string       `json:"status"`
	PromptDigest string       `json:"prompt_digest,omitempty"`
	StartedAt    time.Time    `json:"started_at,omitzero"`
	FinishedAt   time.Time    `json:"finished_at,omitzero"`
	Rows         []CompareRow `json:"rows"`
}

type CompareRunBundle struct {
	CompareRun
	AgentDirs map[string]string `json:"-"`
}

func NewCompareRunStore(root string) *CompareRunStore {
	return &CompareRunStore{root: root}
}

func (s *CompareRunStore) Save(run CompareRun) (CompareRunBundle, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return CompareRunBundle{}, errors.New("compare run root is required")
	}
	run.RunID = strings.TrimSpace(run.RunID)
	if run.RunID == "" {
		run.RunID = newCompareRunID(run.FinishedAt)
	}
	if run.Status == "" {
		run.Status = "completed"
	}
	run.RunDir = filepath.Join(s.root, safeCompareSegment(run.RunID))
	if err := os.MkdirAll(run.RunDir, 0o755); err != nil {
		return CompareRunBundle{}, err
	}
	bundle := CompareRunBundle{CompareRun: run, AgentDirs: make(map[string]string)}
	usedDirs := map[string]int{}
	for i := range run.Rows {
		row := &run.Rows[i]
		if len(row.Events) == 0 {
			continue
		}
		dir := uniqueCompareAgentDir(row.Agent, usedDirs)
		key := row.Agent
		if _, exists := bundle.AgentDirs[key]; exists {
			key = fmt.Sprintf("%s#%d", row.Agent, usedDirs[safeCompareSegment(row.Agent)])
		}
		bundle.AgentDirs[key] = dir
		if err := writeCompareEventsFile(filepath.Join(run.RunDir, "agents", dir, "events.ndjson"), row.Events); err != nil {
			return CompareRunBundle{}, err
		}
	}
	run.Rows = stripCompareRowEvents(run.Rows)
	bundle.Rows = run.Rows
	if err := writeJSONFileAtomic(filepath.Join(run.RunDir, "compare.json"), run, 0o600); err != nil {
		return CompareRunBundle{}, err
	}
	return bundle, nil
}

func stripCompareRowEvents(rows []CompareRow) []CompareRow {
	clean := make([]CompareRow, len(rows))
	copy(clean, rows)
	for i := range clean {
		clean[i].Events = nil
	}
	return clean
}

func newCompareRunID(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return "compare-" + t.UTC().Format("20060102T150405Z")
}

func safeCompareSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	safe := true
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		safe = false
		break
	}
	if safe && value != "." && value != ".." {
		return value
	}
	return storeKey(value)
}

func uniqueCompareAgentDir(agent string, used map[string]int) string {
	base := safeCompareSegment(agent)
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, used[base])
}

func writeCompareEventsFile(path string, events []EventLogLine) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	encoder := json.NewEncoder(f)
	for _, event := range events {
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			return err
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	return f.Sync()
}
