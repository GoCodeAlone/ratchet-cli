package acpclient

import (
	"context"
	"errors"
	"strings"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

const comparePreviewChars = 200

type CompareAgent struct {
	Name string
	Spec AgentSpec
}

type CompareOptions struct {
	Cwd     string
	Timeout time.Duration
	Runner  CompareRunner
}

type CompareRow struct {
	Agent      string         `json:"agent"`
	Status     string         `json:"status"`
	WallMS     int64          `json:"wall_ms"`
	StopReason string         `json:"stop_reason,omitempty"`
	Final      string         `json:"final,omitempty"`
	Error      string         `json:"error,omitempty"`
	Events     []EventLogLine `json:"-"`
}

type CompareRunner interface {
	RunPrompt(context.Context, AgentSpec, RunOptions, string) (Result, error)
}

func Compare(ctx context.Context, agents []CompareAgent, prompt string, opts CompareOptions) ([]CompareRow, error) {
	if len(agents) == 0 {
		return nil, errors.New("at least one compare agent is required")
	}
	runner := opts.Runner
	if runner == nil {
		runner = defaultCompareRunner{}
	}
	rows := make([]CompareRow, 0, len(agents))
	for _, agent := range agents {
		name := agent.Name
		if name == "" {
			name = agent.Spec.Name
		}
		if name == "" {
			name = agent.Spec.Command
		}
		started := time.Now()
		result, err := runner.RunPrompt(ctx, agent.Spec, RunOptions{
			Cwd:     opts.Cwd,
			Timeout: opts.Timeout,
		}, prompt)
		wallMS := result.Duration.Milliseconds()
		if wallMS == 0 {
			wallMS = time.Since(started).Milliseconds()
		}
		row := CompareRow{Agent: name, WallMS: wallMS}
		if err != nil {
			row.Status = "error"
			row.Error = comparePreview(err.Error())
			rows = append(rows, row)
			continue
		}
		row.Status = compareStatus(result.StopReason)
		row.StopReason = string(result.StopReason)
		row.Final = comparePreview(result.Text)
		row.Events = cloneEvents(result.Events)
		rows = append(rows, row)
	}
	return rows, nil
}

type defaultCompareRunner struct{}

func (defaultCompareRunner) RunPrompt(ctx context.Context, spec AgentSpec, opts RunOptions, prompt string) (Result, error) {
	client, err := Start(ctx, spec, opts)
	if err != nil {
		return Result{}, err
	}
	defer client.Close() //nolint:errcheck
	return client.RunPrompt(ctx, prompt)
}

func compareStatus(stopReason acpsdk.StopReason) string {
	if stopReason == acpsdk.StopReasonCancelled {
		return "canceled"
	}
	return "ok"
}

func comparePreview(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= comparePreviewChars {
		return text
	}
	return string([]rune(text)[:comparePreviewChars])
}
