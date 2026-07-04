package acpclient

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	acpx "github.com/GoCodeAlone/acpx-go"
)

const FlowReplayBundleSchema = acpx.SchemaFlowRunBundleV1

var errNotUpstreamACPXBundle = errors.New("not an upstream-shaped acpx bundle")

type FlowReplaySummary struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	ManifestPath string `json:"manifest_path"`
	StepCount    int    `json:"step_count"`
	TraceCount   int    `json:"trace_count"`
	SessionCount int    `json:"session_count"`
}

type FlowRunManifest struct {
	Schema      string                  `json:"schema"`
	RunID       string                  `json:"run_id"`
	Status      string                  `json:"status"`
	Definition  string                  `json:"definition"`
	Input       string                  `json:"input"`
	State       string                  `json:"state"`
	Trace       string                  `json:"trace"`
	Projections FlowReplayProjections   `json:"projections"`
	Steps       []FlowReplayStep        `json:"steps"`
	Sessions    []FlowReplaySession     `json:"sessions,omitempty"`
	Artifacts   []FlowReplayArtifactRef `json:"artifacts,omitempty"`
}

type FlowReplayProjections struct {
	Run   string `json:"run"`
	Live  string `json:"live"`
	Steps string `json:"steps"`
}

type FlowReplayArtifactRef struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`
}

type FlowReplayStep struct {
	NodeID         string                `json:"node_id"`
	Type           string                `json:"type"`
	Status         string                `json:"status"`
	Error          string                `json:"error,omitempty"`
	Output         string                `json:"output,omitempty"`
	OutputArtifact FlowReplayArtifactRef `json:"output_artifact,omitzero"`
	StdoutArtifact FlowReplayArtifactRef `json:"stdout_artifact,omitzero"`
	StderrArtifact FlowReplayArtifactRef `json:"stderr_artifact,omitzero"`
	PromptArtifact FlowReplayArtifactRef `json:"prompt_artifact,omitzero"`
}

type FlowReplaySession struct {
	Handle    string `json:"handle"`
	SessionID string `json:"session_id,omitempty"`
	Events    string `json:"events"`
}

type FlowTraceEvent struct {
	Seq      int       `json:"seq"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	NodeID   string    `json:"node_id"`
	Type     string    `json:"type"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Output   string    `json:"output,omitempty"`
	Session  string    `json:"session,omitempty"`
	Artifact string    `json:"artifact,omitempty"`
}

type flowReplayRecorder struct {
	store     *FlowRunStore
	runID     string
	flowName  string
	startedAt time.Time
	steps     []FlowReplayStep
	acpxSteps []acpx.FlowStepRecord
	sessions  map[string]FlowReplaySession
	events    map[string][]EventLogLine
	prompts   map[string]FlowReplayArtifactRef
	artifacts []FlowReplayArtifactRef
	seq       int
}

func newFlowReplayRecorder(store *FlowRunStore, runID string, def FlowDefinition) *flowReplayRecorder {
	if store == nil {
		return nil
	}
	return &flowReplayRecorder{
		store:     store,
		runID:     runID,
		flowName:  firstNonEmpty(def.Name, "flow"),
		startedAt: time.Now().UTC(),
		sessions:  map[string]FlowReplaySession{},
		events:    map[string][]EventLogLine{},
		prompts:   map[string]FlowReplayArtifactRef{},
	}
}

func (r *flowReplayRecorder) RecordRunStarted() error {
	if r == nil {
		return nil
	}
	return r.appendTraceEvent(acpx.TraceEvent{
		At:      r.startedAt.Format(time.RFC3339Nano),
		Scope:   acpx.TraceScopeRun,
		Type:    "run_started",
		RunID:   r.runID,
		Payload: map[string]json.RawMessage{},
	})
}

func (r *flowReplayRecorder) RecordStep(node FlowNode, step FlowStepRecord) error {
	if r == nil {
		return nil
	}
	replayStep := FlowReplayStep{
		NodeID: step.NodeID,
		Type:   node.Type,
		Status: step.Status,
		Error:  step.Error,
		Output: filepath.ToSlash(filepath.Join("steps", step.NodeID+".json")),
	}
	if len(step.Output) > 0 {
		ref, err := r.writeArtifact("output", []byte(step.Output))
		if err != nil {
			return err
		}
		replayStep.OutputArtifact = ref
	}
	if node.Type == FlowNodeTypeAction && len(step.Output) > 0 {
		var action struct {
			Stdout string `json:"stdout"`
			Stderr string `json:"stderr"`
		}
		if err := json.Unmarshal(step.Output, &action); err == nil {
			if action.Stdout != "" {
				ref, err := r.writeArtifact("stdout", []byte(action.Stdout))
				if err != nil {
					return err
				}
				replayStep.StdoutArtifact = ref
			}
			if action.Stderr != "" {
				ref, err := r.writeArtifact("stderr", []byte(action.Stderr))
				if err != nil {
					return err
				}
				replayStep.StderrArtifact = ref
			}
		}
	}
	if node.Type == FlowNodeTypeACP {
		handle := node.Session
		if handle == "" {
			handle = node.ID
		}
		replayStep.PromptArtifact = r.prompts[handle]
	}
	r.steps = append(r.steps, replayStep)
	now := time.Now().UTC()
	attemptID := step.NodeID + "#1"
	outcome := acpx.NodeOutcomeOK
	if step.Status == FlowStepStatusFailed {
		outcome = acpx.NodeOutcomeFailed
	}
	acpxStep := acpx.FlowStepRecord{
		AttemptID:  attemptID,
		NodeID:     step.NodeID,
		NodeType:   acpx.NodeType(node.Type),
		Outcome:    outcome,
		StartedAt:  now.Format(time.RFC3339Nano),
		FinishedAt: now.Format(time.RFC3339Nano),
		Output:     step.Output,
		Error:      step.Error,
	}
	r.acpxSteps = append(r.acpxSteps, acpxStep)
	if err := r.appendTraceEvent(acpx.TraceEvent{
		At:        now.Format(time.RFC3339Nano),
		Scope:     acpx.TraceScopeNode,
		Type:      "node_started",
		RunID:     r.runID,
		NodeID:    step.NodeID,
		AttemptID: attemptID,
		Payload:   map[string]json.RawMessage{"nodeType": rawJSONString(node.Type)},
	}); err != nil {
		return err
	}
	return r.appendTraceEvent(acpx.TraceEvent{
		At:        now.Format(time.RFC3339Nano),
		Scope:     acpx.TraceScopeNode,
		Type:      "node_outcome",
		RunID:     r.runID,
		NodeID:    step.NodeID,
		AttemptID: attemptID,
		Artifact:  acpxArtifactRef(replayStep.OutputArtifact),
		Payload: map[string]json.RawMessage{
			"outcome": rawJSONString(string(outcome)),
			"status":  rawJSONString(step.Status),
		},
	})
}

func (r *flowReplayRecorder) RecordACPPrompt(handle, sessionID, prompt string, events []EventLogLine) error {
	if r == nil {
		return nil
	}
	ref, err := r.writeArtifact("prompt", []byte(prompt))
	if err != nil {
		return err
	}
	r.prompts[handle] = ref
	eventPath := filepath.ToSlash(filepath.Join("sessions", safeCompareSegment(handle), "events.ndjson"))
	r.events[handle] = appendFlowEvents(r.events[handle], events)
	if err := writeCompareEventsFile(filepath.Join(r.store.Dir(), eventPath), r.events[handle]); err != nil {
		return err
	}
	sessionDir := filepath.ToSlash(filepath.Join("sessions", safeCompareSegment(handle)))
	bindingPath := filepath.ToSlash(filepath.Join(sessionDir, "binding.json"))
	recordPath := filepath.ToSlash(filepath.Join(sessionDir, "record.json"))
	if err := writeJSONFileAtomic(filepath.Join(r.store.Dir(), filepath.FromSlash(bindingPath)), map[string]any{
		"schema":    acpx.SchemaSessionV1,
		"handle":    handle,
		"sessionId": sessionID,
	}, 0o600); err != nil {
		return err
	}
	if err := writeJSONFileAtomic(filepath.Join(r.store.Dir(), filepath.FromSlash(recordPath)), map[string]any{
		"schema":    acpx.SchemaSessionV1,
		"handle":    handle,
		"sessionId": sessionID,
	}, 0o600); err != nil {
		return err
	}
	r.sessions[handle] = FlowReplaySession{Handle: handle, SessionID: sessionID, Events: eventPath}
	return nil
}

func (r *flowReplayRecorder) Finalize(result FlowRunResult) error {
	if r == nil {
		return nil
	}
	sessions := make([]FlowReplaySession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	finishedAt := time.Now().UTC()
	status := acpx.RunStatus(result.Status)
	runProjection := acpx.FlowRunState{
		RunID:      result.RunID,
		FlowName:   r.flowName,
		StartedAt:  r.startedAt.Format(time.RFC3339Nano),
		FinishedAt: finishedAt.Format(time.RFC3339Nano),
		UpdatedAt:  finishedAt.Format(time.RFC3339Nano),
		Status:     status,
		Outputs:    result.Outputs,
		Results:    map[string]acpx.FlowNodeResult{},
	}
	for _, step := range r.acpxSteps {
		runProjection.Results[step.NodeID] = acpx.FlowNodeResult{
			AttemptID:  step.AttemptID,
			NodeID:     step.NodeID,
			NodeType:   step.NodeType,
			Outcome:    step.Outcome,
			StartedAt:  step.StartedAt,
			FinishedAt: step.FinishedAt,
			Output:     step.Output,
			Error:      step.Error,
		}
	}
	if err := r.store.WriteProjection("run", runProjection); err != nil {
		return err
	}
	if err := r.store.WriteProjection("live", runProjection); err != nil {
		return err
	}
	if err := r.store.WriteProjection("steps", r.acpxSteps); err != nil {
		return err
	}
	manifestSessions := make([]acpx.ManifestSessionEntry, 0, len(sessions))
	for _, session := range sessions {
		sessionDir := filepath.ToSlash(filepath.Join(acpx.DefaultSessionsDir, safeCompareSegment(session.Handle)))
		manifestSessions = append(manifestSessions, acpx.ManifestSessionEntry{
			ID:          safeCompareSegment(session.Handle),
			Handle:      session.Handle,
			BindingPath: filepath.ToSlash(filepath.Join(sessionDir, "binding.json")),
			RecordPath:  filepath.ToSlash(filepath.Join(sessionDir, "record.json")),
			EventsPath:  session.Events,
		})
	}
	return r.store.WriteACPXManifest(acpx.Manifest{
		Schema:      acpx.SchemaFlowRunBundleV1,
		RunID:       result.RunID,
		FlowName:    r.flowName,
		StartedAt:   r.startedAt.Format(time.RFC3339Nano),
		FinishedAt:  finishedAt.Format(time.RFC3339Nano),
		Status:      status,
		TraceSchema: acpx.SchemaFlowTraceEventV1,
		Paths: acpx.ManifestPaths{
			Flow:            acpx.DefaultFlowPath,
			Trace:           acpx.DefaultTracePath,
			RunProjection:   acpx.DefaultRunProjectionPath,
			LiveProjection:  acpx.DefaultLiveProjectionPath,
			StepsProjection: acpx.DefaultStepsProjectionPath,
			SessionsDir:     acpx.DefaultSessionsDir,
			ArtifactsDir:    acpx.DefaultArtifactsDir,
		},
		Sessions: manifestSessions,
	})
}

func (r *flowReplayRecorder) writeArtifact(kind string, data []byte) (FlowReplayArtifactRef, error) {
	ref, err := r.store.WriteArtifact(kind, data)
	if err != nil {
		return FlowReplayArtifactRef{}, err
	}
	r.artifacts = append(r.artifacts, ref)
	return ref, nil
}

func appendFlowEvents(existing, next []EventLogLine) []EventLogLine {
	out := make([]EventLogLine, 0, len(existing)+len(next))
	out = append(out, existing...)
	for _, event := range next {
		event.Seq = len(out) + 1
		out = append(out, event)
	}
	return out
}

func (s *FlowRunStore) WriteArtifact(kind string, data []byte) (FlowReplayArtifactRef, error) {
	if s == nil {
		return FlowReplayArtifactRef{}, errors.New("flow run store is required")
	}
	sum := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", sum[:])
	rel := filepath.ToSlash(filepath.Join("artifacts", "sha256", hash))
	path := filepath.Join(s.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return FlowReplayArtifactRef{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return FlowReplayArtifactRef{}, err
	}
	return FlowReplayArtifactRef{Kind: kind, SHA256: "sha256:" + hash, Path: rel}, nil
}

func (r *flowReplayRecorder) appendTraceEvent(event acpx.TraceEvent) error {
	r.seq++
	event.Seq = int64(r.seq)
	if event.Payload == nil {
		event.Payload = map[string]json.RawMessage{}
	}
	return r.store.AppendTraceEvent(event)
}

func (s *FlowRunStore) AppendTraceEvent(event acpx.TraceEvent) (err error) {
	if s == nil {
		return errors.New("flow run store is required")
	}
	path := filepath.Join(s.dir, "trace.ndjson")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	return json.NewEncoder(f).Encode(event)
}

func (s *FlowRunStore) WriteProjection(name string, value any) error {
	if !safeFlowIdentifier(name) {
		return errors.New("flow projection name must be a safe path segment")
	}
	return writeJSONFileAtomic(filepath.Join(s.dir, "projections", name+".json"), value, 0o600)
}

func (s *FlowRunStore) WriteManifest(manifest FlowRunManifest) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "manifest.json"), manifest, 0o600)
}

func (s *FlowRunStore) WriteACPXManifest(manifest acpx.Manifest) error {
	return writeJSONFileAtomic(filepath.Join(s.dir, "manifest.json"), manifest, 0o600)
}

func rawJSONString(value string) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return data
}

func acpxArtifactRef(ref FlowReplayArtifactRef) *acpx.ArtifactRef {
	if ref.Path == "" {
		return nil
	}
	return &acpx.ArtifactRef{
		Path:      ref.Path,
		MediaType: "application/json",
		SHA256:    strings.TrimPrefix(ref.SHA256, "sha256:"),
	}
}

func LoadFlowReplaySummary(runDir string) (FlowReplaySummary, error) {
	runDir = filepath.Clean(runDir)
	if summary, err := loadACPXFlowReplaySummary(runDir); err == nil {
		return summary, nil
	} else if !errors.Is(err, errNotUpstreamACPXBundle) {
		return FlowReplaySummary{}, err
	}
	manifestPath := filepath.Join(runDir, "manifest.json")
	var manifest FlowRunManifest
	if err := readJSONFile(manifestPath, &manifest); err != nil {
		return FlowReplaySummary{}, err
	}
	if manifest.Schema != FlowReplayBundleSchema {
		return FlowReplaySummary{}, fmt.Errorf("unsupported flow replay schema %q", manifest.Schema)
	}
	paths := []string{manifest.Definition, manifest.Input, manifest.State, manifest.Trace}
	paths = append(paths, manifest.Projections.Run, manifest.Projections.Live, manifest.Projections.Steps)
	for _, step := range manifest.Steps {
		paths = append(paths, step.Output, step.OutputArtifact.Path, step.StdoutArtifact.Path, step.StderrArtifact.Path, step.PromptArtifact.Path)
	}
	for _, session := range manifest.Sessions {
		paths = append(paths, session.Events)
	}
	for _, rel := range paths {
		if rel == "" {
			continue
		}
		if _, err := containedReplayPath(runDir, rel); err != nil {
			return FlowReplaySummary{}, err
		}
	}
	tracePath, err := containedReplayPath(runDir, manifest.Trace)
	if err != nil {
		return FlowReplaySummary{}, err
	}
	traceCount, err := countNDJSONLines(tracePath)
	if err != nil {
		return FlowReplaySummary{}, err
	}
	stepCount := len(manifest.Steps)
	if stepCount == 0 && manifest.Projections.Steps != "" {
		var steps []FlowReplayStep
		if err := readJSONFile(filepath.Join(runDir, filepath.FromSlash(manifest.Projections.Steps)), &steps); err == nil {
			stepCount = len(steps)
		}
	}
	return FlowReplaySummary{
		RunID:        manifest.RunID,
		Status:       manifest.Status,
		ManifestPath: "manifest.json",
		StepCount:    stepCount,
		TraceCount:   traceCount,
		SessionCount: len(manifest.Sessions),
	}, nil
}

func loadACPXFlowReplaySummary(runDir string) (FlowReplaySummary, error) {
	if err := validateUpstreamACPXReplayPaths(runDir); err != nil {
		return FlowReplaySummary{}, err
	}
	bundle, err := acpx.LoadBundle(context.Background(), runDir)
	if err != nil {
		return FlowReplaySummary{}, err
	}
	projection, err := acpx.RebuildTraceProjection(bundle.Trace)
	if err != nil {
		return FlowReplaySummary{}, err
	}
	return FlowReplaySummary{
		RunID:        bundle.Manifest.RunID,
		Status:       string(bundle.Manifest.Status),
		ManifestPath: "manifest.json",
		StepCount:    len(projection.Attempts),
		TraceCount:   len(bundle.Trace),
		SessionCount: len(bundle.Manifest.Sessions),
	}, nil
}

func validateUpstreamACPXReplayPaths(runDir string) error {
	var manifest struct {
		RunID string `json:"runId"`
		Paths struct {
			Flow            string `json:"flow"`
			Trace           string `json:"trace"`
			RunProjection   string `json:"runProjection"`
			LiveProjection  string `json:"liveProjection"`
			StepsProjection string `json:"stepsProjection"`
			SessionsDir     string `json:"sessionsDir"`
			ArtifactsDir    string `json:"artifactsDir"`
		} `json:"paths"`
		Sessions []struct {
			BindingPath string `json:"bindingPath"`
			RecordPath  string `json:"recordPath"`
			EventsPath  string `json:"eventsPath"`
		} `json:"sessions"`
	}
	if err := readJSONFile(filepath.Join(runDir, "manifest.json"), &manifest); err != nil {
		return err
	}
	if manifest.RunID == "" && manifest.Paths.Trace == "" {
		return errNotUpstreamACPXBundle
	}
	paths := []string{
		manifest.Paths.Flow,
		manifest.Paths.Trace,
		manifest.Paths.RunProjection,
		manifest.Paths.LiveProjection,
		manifest.Paths.StepsProjection,
		manifest.Paths.SessionsDir,
		manifest.Paths.ArtifactsDir,
	}
	for _, session := range manifest.Sessions {
		paths = append(paths, session.BindingPath, session.RecordPath, session.EventsPath)
	}
	for _, rel := range paths {
		if rel == "" {
			continue
		}
		if _, err := containedReplayPath(runDir, rel); err != nil {
			return err
		}
	}
	return nil
}

func containedReplayPath(runDir, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", fmt.Errorf("flow replay path %q is outside run dir", rel)
	}
	base := filepath.Clean(runDir)
	path := filepath.Clean(filepath.Join(base, filepath.FromSlash(rel)))
	if !pathWithin(base, path) {
		return "", fmt.Errorf("flow replay path %q is outside run dir", rel)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if !pathWithin(realPathOrClean(base), filepath.Clean(resolvedPath)) {
		return "", fmt.Errorf("flow replay path %q is outside run dir", rel)
	}
	return path, nil
}

func countNDJSONLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count, scanner.Err()
}
