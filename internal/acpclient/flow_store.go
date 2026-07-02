package acpclient

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type FlowRunStore struct {
	dir string
}

func NewFlowRunStore(rootDir, runID string) (*FlowRunStore, error) {
	rootDir = strings.TrimSpace(rootDir)
	runID = strings.TrimSpace(runID)
	if rootDir == "" {
		return nil, errors.New("flow run root directory is required")
	}
	if runID == "" {
		return nil, errors.New("flow run id is required")
	}
	return &FlowRunStore{dir: filepath.Join(rootDir, runID)}, nil
}

func (s *FlowRunStore) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *FlowRunStore) WriteDefinition(def FlowDefinition) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "flow.json"), def, 0o600)
}

func (s *FlowRunStore) WriteInput(input any) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "input.json"), input, 0o600)
}

func (s *FlowRunStore) WriteState(state FlowRunState) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "state.json"), state, 0o600)
}

func (s *FlowRunStore) WriteStep(nodeID string, output json.RawMessage) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "steps", nodeID+".json"), json.RawMessage(output), 0o600)
}
