package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

type FlowPromptRunner interface {
	SessionID() acpsdk.SessionId
	Prompt(context.Context, string) (Result, error)
}

type ActionRunner interface {
	RunAction(context.Context, ActionRunOptions) (ActionResult, error)
}

type ActionRunOptions struct {
	Command string
	Args    []string
	Cwd     string
	Env     map[string]string
	Input   json.RawMessage
}

type ActionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

type FlowRunOptions struct {
	RunID              string
	RunRoot            string
	Cwd                string
	DefaultAgent       string
	DefaultCommand     string
	DefaultArgs        []string
	Registry           Registry
	AllowedPermissions []string
	ActionOutputLimit  int
	ActionRunner       ActionRunner
	StartRunner        func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error)
}

type FlowRunResult struct {
	RunID   string                     `json:"run_id"`
	Status  string                     `json:"status"`
	RunDir  string                     `json:"run_dir,omitempty"`
	Outputs map[string]json.RawMessage `json:"outputs"`
	Steps   []FlowStepRecord           `json:"steps"`
}

const defaultActionOutputLimit = 64 * 1024

func RunFlow(ctx context.Context, def FlowDefinition, input map[string]any, opts FlowRunOptions) (FlowRunResult, error) {
	if err := def.Validate(); err != nil {
		return FlowRunResult{}, err
	}
	allowed := flowPermissionSet(opts.AllowedPermissions)
	if err := preflightFlowPermissions(def, opts, allowed); err != nil {
		return FlowRunResult{}, err
	}
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		runID = fmt.Sprintf("flow-%d", time.Now().UTC().UnixNano())
	}
	result := FlowRunResult{
		RunID:   runID,
		Status:  FlowRunStatusRunning,
		Outputs: map[string]json.RawMessage{},
	}
	var store *FlowRunStore
	var replay *flowReplayRecorder
	if opts.RunRoot != "" {
		var err error
		store, err = NewFlowRunStore(opts.RunRoot, runID)
		if err != nil {
			return result, err
		}
		result.RunDir = store.Dir()
		if err := store.WriteDefinition(def); err != nil {
			return result, err
		}
		if err := store.WriteInput(input); err != nil {
			return result, err
		}
		replay = newFlowReplayRecorder(store, runID)
	}

	nodes := flowNodeMap(def.Nodes)
	outputValues := map[string]any{}
	runners := map[string]FlowPromptRunner{}
	closers := []func() error{}
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			_ = closers[i]()
		}
	}()

	for _, nodeID := range flowExecutionOrder(def) {
		node := nodes[nodeID]
		output, err := runFlowNode(ctx, node, input, outputValues, runners, &closers, opts, replay)
		step := FlowStepRecord{NodeID: node.ID, Type: node.Type, Output: output}
		if err != nil {
			step.Status = FlowStepStatusFailed
			step.Error = err.Error()
			result.Status = FlowRunStatusFailed
			result.Steps = append(result.Steps, step)
			if store != nil {
				if writeErr := store.WriteStep(node.ID, output); writeErr != nil {
					return result, writeErr
				}
				if recErr := replay.RecordStep(node, step); recErr != nil {
					return result, recErr
				}
			}
			if stateErr := persistFlowState(store, result); stateErr != nil {
				return result, stateErr
			}
			if finalizeErr := replay.Finalize(result); finalizeErr != nil {
				return result, finalizeErr
			}
			return result, err
		}
		step.Status = FlowStepStatusCompleted
		result.Outputs[node.ID] = output
		var decoded any
		if err := json.Unmarshal(output, &decoded); err == nil {
			outputValues[node.ID] = decoded
		}
		result.Steps = append(result.Steps, step)
		if store != nil {
			if err := store.WriteStep(node.ID, output); err != nil {
				return result, err
			}
			if err := replay.RecordStep(node, step); err != nil {
				return result, err
			}
			if err := persistFlowState(store, result); err != nil {
				return result, err
			}
		}
	}
	result.Status = FlowRunStatusCompleted
	if err := persistFlowState(store, result); err != nil {
		return result, err
	}
	if err := replay.Finalize(result); err != nil {
		return result, err
	}
	return result, nil
}

func runFlowNode(ctx context.Context, node FlowNode, input map[string]any, outputs map[string]any, runners map[string]FlowPromptRunner, closers *[]func() error, opts FlowRunOptions, replay *flowReplayRecorder) (json.RawMessage, error) {
	switch node.Type {
	case FlowNodeTypeCompute:
		return runComputeFlowNode(node, outputs)
	case FlowNodeTypeAction:
		return runActionFlowNode(ctx, node, opts)
	case FlowNodeTypeACP:
		return runACPFlowNode(ctx, node, input, outputs, runners, closers, opts, replay)
	default:
		return nil, fmt.Errorf("%w: unknown node type %s", ErrInvalidFlowDefinition, node.Type)
	}
}

func runACPFlowNode(ctx context.Context, node FlowNode, input map[string]any, outputs map[string]any, runners map[string]FlowPromptRunner, closers *[]func() error, opts FlowRunOptions, replay *flowReplayRecorder) (json.RawMessage, error) {
	prompt, err := renderFlowPrompt(node, input, outputs)
	if err != nil {
		return nil, err
	}
	handle := node.Session
	if handle == "" {
		handle = node.ID
	}
	runner := runners[handle]
	if runner == nil {
		spec, runOpts, err := flowAgentSpec(node, opts)
		if err != nil {
			return nil, err
		}
		start := opts.StartRunner
		if start == nil {
			start = defaultFlowStartRunner
		}
		var closeRunner func() error
		runner, closeRunner, err = start(ctx, spec, runOpts, "")
		if err != nil {
			return nil, err
		}
		runners[handle] = runner
		if closeRunner != nil {
			*closers = append(*closers, closeRunner)
		}
	}
	result, err := runner.Prompt(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if events, ok := runner.(interface{ LastEvents() []EventLogLine }); ok {
		if err := replay.RecordACPPrompt(handle, string(result.SessionID), prompt, events.LastEvents()); err != nil {
			return nil, err
		}
	}
	return marshalFlowOutput(map[string]any{
		"text":        result.Text,
		"stop_reason": string(result.StopReason),
		"session_id":  string(result.SessionID),
	})
}

func runActionFlowNode(ctx context.Context, node FlowNode, opts FlowRunOptions) (json.RawMessage, error) {
	runner := opts.ActionRunner
	if runner == nil {
		runner = defaultActionRunner{}
	}
	cwd, err := resolveFlowNodeCWD(node, opts, flowPermissionSet(opts.AllowedPermissions))
	if err != nil {
		return nil, err
	}
	result, err := runner.RunAction(ctx, ActionRunOptions{
		Command: node.Command,
		Args:    node.Args,
		Cwd:     cwd,
		Env:     node.Env,
		Input:   node.Input,
	})
	output, marshalErr := marshalActionOutput(result, cwd, opts.ActionOutputLimit)
	if err != nil {
		return output, err
	}
	if marshalErr != nil {
		return nil, marshalErr
	}
	if result.ExitCode != 0 {
		return output, fmt.Errorf("action %s exited with %d", node.ID, result.ExitCode)
	}
	return output, nil
}

func runComputeFlowNode(node FlowNode, outputs map[string]any) (json.RawMessage, error) {
	if node.Select != "" {
		selected, err := selectFlowOutput(node.Select, outputs)
		if err != nil {
			return nil, err
		}
		return marshalFlowOutput(selected)
	}
	return json.RawMessage(node.Value), nil
}

func renderFlowPrompt(node FlowNode, input map[string]any, outputs map[string]any) (string, error) {
	tmpl, err := template.New(node.ID).Parse(node.Prompt)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Input   map[string]any
		Outputs map[string]any
		Node    FlowNode
	}{Input: input, Outputs: outputs, Node: node})
	return buf.String(), err
}

func flowAgentSpec(node FlowNode, opts FlowRunOptions) (AgentSpec, RunOptions, error) {
	args := opts.DefaultArgs
	if len(node.Args) > 0 {
		args = node.Args
	}
	runOpts := RunOptions{
		Agent:   firstNonEmpty(node.Agent, opts.DefaultAgent),
		Command: firstNonEmpty(node.Command, opts.DefaultCommand),
		Args:    args,
	}
	cwd, err := resolveFlowNodeCWD(node, opts, flowPermissionSet(opts.AllowedPermissions))
	if err != nil {
		return AgentSpec{}, RunOptions{}, err
	}
	runOpts.Cwd = cwd
	reg := opts.Registry
	if reg.specs == nil {
		reg = DefaultRegistry()
	}
	spec, err := reg.Resolve(runOpts)
	return spec, runOpts, err
}

type defaultActionRunner struct{}

func (defaultActionRunner) RunAction(ctx context.Context, opts ActionRunOptions) (ActionResult, error) {
	started := time.Now()
	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	if len(opts.Env) > 0 {
		env := os.Environ()
		for k, v := range opts.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	if len(opts.Input) > 0 {
		cmd.Stdin = bytes.NewReader(opts.Input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ActionResult{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(started),
	}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	result.ExitCode = -1
	return result, err
}

func defaultFlowStartRunner(ctx context.Context, spec AgentSpec, opts RunOptions, existingID string) (FlowPromptRunner, func() error, error) {
	client, err := Start(ctx, spec, opts)
	if err != nil {
		return nil, nil, err
	}
	runner, err := client.StartSession(ctx, existingID)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return runner, client.Close, nil
}

func flowExecutionOrder(def FlowDefinition) []string {
	graph := map[string][]string{}
	indegree := map[string]int{}
	for _, node := range def.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range def.Edges {
		graph[edge.From] = append(graph[edge.From], edge.To)
		indegree[edge.To]++
	}
	ready := []string{def.StartAt}
	queued := map[string]bool{def.StartAt: true}
	order := make([]string, 0, len(def.Nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, next := range graph[id] {
			indegree[next]--
			if indegree[next] == 0 && !queued[next] {
				ready = append(ready, next)
				queued[next] = true
			}
		}
	}
	return order
}

func flowNodeMap(nodes []FlowNode) map[string]FlowNode {
	out := make(map[string]FlowNode, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func persistFlowState(store *FlowRunStore, result FlowRunResult) error {
	if store == nil {
		return nil
	}
	return store.WriteState(FlowRunState{RunID: result.RunID, Status: result.Status, Steps: result.Steps})
}

func preflightFlowPermissions(def FlowDefinition, opts FlowRunOptions, allowed map[string]bool) error {
	required := flowPermissionSet(def.Requires)
	for _, node := range def.Nodes {
		if node.Type == FlowNodeTypeAction {
			required["shell"] = true
		}
		if node.Type == FlowNodeTypeAction || node.Type == FlowNodeTypeACP {
			if _, err := resolveFlowNodeCWD(node, opts, allowed); err != nil {
				if missing, ok := missingFlowPermission(err); ok {
					required[missing] = true
					continue
				}
				return err
			}
		}
	}
	for permission := range required {
		if !allowed[permission] {
			return fmt.Errorf("flow requires permission %s; pass --allow %s", permission, permission)
		}
	}
	return nil
}

func flowPermissionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

type flowMissingPermissionError struct {
	permission string
}

func (e flowMissingPermissionError) Error() string {
	return fmt.Sprintf("flow requires permission %s; pass --allow %s", e.permission, e.permission)
}

func missingFlowPermission(err error) (string, bool) {
	e, ok := err.(flowMissingPermissionError)
	return e.permission, ok
}

func resolveFlowNodeCWD(node FlowNode, opts FlowRunOptions, allowed map[string]bool) (string, error) {
	base := strings.TrimSpace(opts.Cwd)
	if base == "" {
		base = "."
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	baseAbs = filepath.Clean(baseAbs)
	cwd := strings.TrimSpace(node.Cwd)
	if cwd == "" {
		return baseAbs, nil
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(baseAbs, cwd)
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	cwdAbs = filepath.Clean(cwdAbs)
	containmentBase := realPathOrClean(baseAbs)
	containmentCWD := realPathOrClean(cwdAbs)
	if !pathWithin(containmentBase, containmentCWD) && !allowed["outside-cwd"] {
		return "", flowMissingPermissionError{permission: "outside-cwd"}
	}
	return cwdAbs, nil
}

func realPathOrClean(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func pathWithin(base, path string) bool {
	if base == path {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func marshalActionOutput(result ActionResult, cwd string, limit int) (json.RawMessage, error) {
	if limit <= 0 {
		limit = defaultActionOutputLimit
	}
	stdout, stdoutTruncated := truncateRunes(result.Stdout, limit)
	stderr, stderrTruncated := truncateRunes(result.Stderr, limit)
	return marshalFlowOutput(map[string]any{
		"exit_code":        result.ExitCode,
		"stdout":           stdout,
		"stderr":           stderr,
		"stdout_truncated": stdoutTruncated,
		"stderr_truncated": stderrTruncated,
		"duration_ms":      result.Duration.Milliseconds(),
		"cwd":              cwd,
	})
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

func marshalFlowOutput(value any) (json.RawMessage, error) {
	b, err := json.Marshal(value)
	return json.RawMessage(b), err
}

func selectFlowOutput(path string, outputs map[string]any) (any, error) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("flow select path is required")
	}
	current := outputs[parts[0]]
	if current == nil {
		return nil, fmt.Errorf("flow select node %s has no output", parts[0])
	}
	for _, part := range parts[1:] {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("flow select %s is not an object", part)
		}
		current = m[part]
		if current == nil {
			return nil, fmt.Errorf("flow select field %s has no value", part)
		}
	}
	return current, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
