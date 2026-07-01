package acpclient

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientRunPromptCapturesAgentUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentR.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientR.Close()
		_ = agentToClientW.Close()
	})

	agent := &echoAgent{}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	})

	result, err := client.RunPrompt(ctx, "hello")
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.SessionID == "" {
		t.Fatal("SessionID is empty")
	}
	if result.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, acpsdk.StopReasonEndTurn)
	}
	if got, want := result.Text, "echo: hello"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if len(result.Updates) != 1 {
		t.Fatalf("Updates len = %d, want 1", len(result.Updates))
	}
}

func TestClientRunPromptResetsCapturedUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentR.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientR.Close()
		_ = agentToClientW.Close()
	})

	agent := &echoAgent{}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	})

	first, err := client.RunPrompt(ctx, "first")
	if err != nil {
		t.Fatalf("RunPrompt first: %v", err)
	}
	if first.Text != "echo: first" {
		t.Fatalf("first Text = %q", first.Text)
	}

	second, err := client.RunPrompt(ctx, "second")
	if err != nil {
		t.Fatalf("RunPrompt second: %v", err)
	}
	if second.Text != "echo: second" {
		t.Fatalf("second Text = %q, want only second prompt output", second.Text)
	}
	if len(second.Updates) != 1 {
		t.Fatalf("second Updates len = %d, want 1", len(second.Updates))
	}
}

func TestCallbacksEnforceCWDAndWritePolicy(t *testing.T) {
	cwd := t.TempDir()
	inside := filepath.Join(cwd, "notes.txt")
	if err := os.WriteFile(inside, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}

	cb := NewCallbacks(RunOptions{Cwd: cwd})
	read, err := cb.ReadTextFile(t.Context(), acpsdk.ReadTextFileRequest{Path: inside})
	if err != nil {
		t.Fatalf("ReadTextFile inside cwd: %v", err)
	}
	if read.Content != "line one\nline two\n" {
		t.Fatalf("ReadTextFile content = %q", read.Content)
	}

	_, err = cb.ReadTextFile(t.Context(), acpsdk.ReadTextFileRequest{Path: outside})
	if !errors.Is(err, ErrPathOutsideCWD) {
		t.Fatalf("ReadTextFile outside error = %v, want ErrPathOutsideCWD", err)
	}

	_, err = cb.WriteTextFile(t.Context(), acpsdk.WriteTextFileRequest{
		Path:    filepath.Join(cwd, "new.txt"),
		Content: "new",
	})
	if !errors.Is(err, ErrWritesDisabled) {
		t.Fatalf("WriteTextFile error = %v, want ErrWritesDisabled", err)
	}

	writable := NewCallbacks(RunOptions{Cwd: cwd, AllowWrites: true})
	_, err = writable.WriteTextFile(t.Context(), acpsdk.WriteTextFileRequest{
		Path:    filepath.Join(cwd, "new.txt"),
		Content: "new",
	})
	if err != nil {
		t.Fatalf("WriteTextFile with AllowWrites: %v", err)
	}
}

func TestCallbacksRejectSymlinkEscape(t *testing.T) {
	cwd := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}

	link := filepath.Join(cwd, "secret-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cb := NewCallbacks(RunOptions{Cwd: cwd, AllowWrites: true})
	_, err := cb.ReadTextFile(t.Context(), acpsdk.ReadTextFileRequest{Path: link})
	if !errors.Is(err, ErrPathOutsideCWD) {
		t.Fatalf("ReadTextFile symlink escape error = %v, want ErrPathOutsideCWD", err)
	}

	linkDir := filepath.Join(cwd, "outside-dir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	_, err = cb.WriteTextFile(t.Context(), acpsdk.WriteTextFileRequest{
		Path:    filepath.Join(linkDir, "new.txt"),
		Content: "new",
	})
	if !errors.Is(err, ErrPathOutsideCWD) {
		t.Fatalf("WriteTextFile symlink escape error = %v, want ErrPathOutsideCWD", err)
	}
}

type echoAgent struct {
	conn *acpsdk.AgentSideConnection
}

var _ acpsdk.Agent = (*echoAgent)(nil)

func (*echoAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*echoAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		AgentInfo: &acpsdk.Implementation{Name: "echo-agent", Version: "test"},
	}, nil
}

func (*echoAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (*echoAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "session-echo"}, nil
}

func (a *echoAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	var prompt strings.Builder
	for _, block := range params.Prompt {
		if block.Text != nil {
			prompt.WriteString(block.Text.Text)
		}
	}
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: params.SessionId,
		Update:    acpsdk.UpdateAgentMessageText("echo: " + prompt.String()),
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*echoAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}
