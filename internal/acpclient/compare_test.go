package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestCompareRunsAgentsSeriallyAndCapturesSuccessRows(t *testing.T) {
	runner := &fakeCompareRunner{
		results: map[string]Result{
			"first":  {StopReason: acpsdk.StopReasonEndTurn, Text: "first final\nmessage", Duration: 10 * time.Millisecond},
			"second": {StopReason: acpsdk.StopReasonMaxTokens, Text: "second final message", Duration: 20 * time.Millisecond},
		},
	}

	rows, err := Compare(t.Context(), []CompareAgent{
		{Name: "first", Spec: AgentSpec{Name: "first", Command: "first"}},
		{Name: "second", Spec: AgentSpec{Name: "second", Command: "second"}},
	}, "shared prompt", CompareOptions{Runner: runner, Cwd: "/tmp/project", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got, want := strings.Join(runner.calls, ","), "first:shared prompt,second:shared prompt"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want 2 rows", rows)
	}
	if rows[0].Agent != "first" || rows[0].Status != "ok" || rows[0].StopReason != "end_turn" ||
		rows[0].WallMS != 10 || rows[0].Final != "first final message" || rows[0].Error != "" {
		t.Fatalf("first row = %#v", rows[0])
	}
	if rows[1].Agent != "second" || rows[1].Status != "ok" || rows[1].StopReason != "max_tokens" ||
		rows[1].WallMS != 20 || rows[1].Final != "second final message" {
		t.Fatalf("second row = %#v", rows[1])
	}
}

func TestCompareCapturesErrorsAndContinuesLaterRows(t *testing.T) {
	runner := &fakeCompareRunner{
		results: map[string]Result{
			"later": {StopReason: acpsdk.StopReasonEndTurn, Text: "later final"},
		},
		errors: map[string]error{
			"broken": errors.New("broken agent failed"),
		},
	}

	rows, err := Compare(t.Context(), []CompareAgent{
		{Name: "broken", Spec: AgentSpec{Name: "broken", Command: "broken"}},
		{Name: "later", Spec: AgentSpec{Name: "later", Command: "later"}},
	}, "prompt", CompareOptions{Runner: runner})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got, want := strings.Join(runner.calls, ","), "broken:prompt,later:prompt"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want 2 rows", rows)
	}
	if rows[0].Status != "error" || !strings.Contains(rows[0].Error, "broken agent failed") {
		t.Fatalf("broken row = %#v", rows[0])
	}
	if rows[1].Status != "ok" || rows[1].Final != "later final" {
		t.Fatalf("later row = %#v", rows[1])
	}
}

func TestCompareCapturesResultEventsForSavedBundles(t *testing.T) {
	events := []EventLogLine{{
		Seq:       1,
		At:        time.Date(2026, 7, 3, 9, 30, 0, 0, time.UTC),
		Direction: EventDirectionOutbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"first"}}`),
	}}
	runner := &fakeCompareRunner{
		results: map[string]Result{
			"first": {StopReason: acpsdk.StopReasonEndTurn, Text: "first final", Events: events},
		},
	}

	rows, err := Compare(t.Context(), []CompareAgent{
		{Name: "first", Spec: AgentSpec{Name: "first", Command: "first"}},
		{Name: "second", Spec: AgentSpec{Name: "second", Command: "second"}},
	}, "shared prompt", CompareOptions{Runner: runner})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want 2 rows", rows)
	}
	if len(rows[0].Events) != 1 || rows[0].Events[0].Direction != EventDirectionOutbound {
		t.Fatalf("first row events = %#v, want captured outbound event", rows[0].Events)
	}
	if len(rows[1].Events) != 0 {
		t.Fatalf("second row events = %#v, want none", rows[1].Events)
	}
}

func TestCompareMapsCancelledStopReasonToCanceledStatus(t *testing.T) {
	runner := &fakeCompareRunner{
		results: map[string]Result{
			"cancel": {StopReason: acpsdk.StopReasonCancelled, Text: "partial"},
		},
	}

	rows, err := Compare(t.Context(), []CompareAgent{
		{Name: "cancel", Spec: AgentSpec{Name: "cancel", Command: "cancel"}},
	}, "prompt", CompareOptions{Runner: runner})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != "canceled" || rows[0].StopReason != "cancelled" {
		t.Fatalf("rows = %#v, want canceled row", rows)
	}
}

func TestComparePreviewTruncatesByRune(t *testing.T) {
	preview := comparePreview(strings.Repeat("é", comparePreviewChars+1))
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is not valid UTF-8: %q", preview)
	}
	if got := utf8.RuneCountInString(preview); got != comparePreviewChars {
		t.Fatalf("preview rune count = %d, want %d", got, comparePreviewChars)
	}
}

type fakeCompareRunner struct {
	results map[string]Result
	errors  map[string]error
	calls   []string
}

func (r *fakeCompareRunner) RunPrompt(_ context.Context, spec AgentSpec, _ RunOptions, prompt string) (Result, error) {
	r.calls = append(r.calls, spec.Name+":"+prompt)
	if r.errors != nil {
		if err := r.errors[spec.Name]; err != nil {
			return Result{}, err
		}
	}
	if r.results != nil {
		if result, ok := r.results[spec.Name]; ok {
			return result, nil
		}
	}
	return Result{}, nil
}
