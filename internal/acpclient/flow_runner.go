package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

type FlowPromptRunner interface {
	SessionID() acpsdk.SessionId
	Prompt(context.Context, string) (Result, error)
}

type FlowRunOptions struct {
	RunID          string
	RunRoot        string
	Cwd            string
	DefaultAgent   string
	DefaultCommand string
	DefaultArgs    []string
	StartRunner    func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error)
}

type FlowRunResult struct {
	RunID   string                     `json:"run_id"`
	Status  string                     `json:"status"`
	RunDir  string                     `json:"run_dir,omitempty"`
	Outputs map[string]json.RawMessage `json:"outputs"`
	Steps   []FlowStepRecord           `json:"steps"`
}

func RunFlow(ctx context.Context, def FlowDefinition, input map[string]any, opts FlowRunOptions) (FlowRunResult, error) {
	if err := def.Validate(); err != nil {
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
		output, err := runFlowNode(ctx, node, input, outputValues, runners, &closers, opts)
		step := FlowStepRecord{NodeID: node.ID, Type: node.Type, Output: output}
		if err != nil {
			step.Status = FlowStepStatusFailed
			step.Error = err.Error()
			result.Status = FlowRunStatusFailed
			result.Steps = append(result.Steps, step)
			_ = persistFlowState(store, result)
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
			if err := persistFlowState(store, result); err != nil {
				return result, err
			}
		}
	}
	result.Status = FlowRunStatusCompleted
	if err := persistFlowState(store, result); err != nil {
		return result, err
	}
	return result, nil
}

func runFlowNode(ctx context.Context, node FlowNode, input map[string]any, outputs map[string]any, runners map[string]FlowPromptRunner, closers *[]func() error, opts FlowRunOptions) (json.RawMessage, error) {
	switch node.Type {
	case FlowNodeTypeCompute:
		return runComputeFlowNode(node, outputs)
	case FlowNodeTypeACP:
		return runACPFlowNode(ctx, node, input, outputs, runners, closers, opts)
	default:
		return nil, fmt.Errorf("%w: unknown node type %s", ErrInvalidFlowDefinition, node.Type)
	}
}

func runACPFlowNode(ctx context.Context, node FlowNode, input map[string]any, outputs map[string]any, runners map[string]FlowPromptRunner, closers *[]func() error, opts FlowRunOptions) (json.RawMessage, error) {
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
	return marshalFlowOutput(map[string]any{
		"text":        result.Text,
		"stop_reason": string(result.StopReason),
		"session_id":  string(result.SessionID),
	})
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
		Cwd:     opts.Cwd,
	}
	spec, err := DefaultRegistry().Resolve(runOpts)
	return spec, runOpts, err
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
	for _, edge := range def.Edges {
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	seen := map[string]bool{}
	var order []string
	var walk func(string)
	walk = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		order = append(order, id)
		for _, next := range graph[id] {
			walk(next)
		}
	}
	walk(def.StartAt)
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
