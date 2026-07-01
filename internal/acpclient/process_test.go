package acpclient

import (
	"context"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientRunPromptAgainstFixtureProcess(t *testing.T) {
	fixture := BuildFixtureAgent(t)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	client, err := Start(ctx, AgentSpec{Name: "fixture", Command: fixture}, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	result, err := client.RunPrompt(ctx, "process hello")
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, acpsdk.StopReasonEndTurn)
	}
	if got, want := result.Text, "fixture: process hello"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}
