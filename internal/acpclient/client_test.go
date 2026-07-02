package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

func TestClientRunPromptCapturesEventLog(t *testing.T) {
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
	if len(result.Events) < 3 {
		t.Fatalf("Events len = %d, want outbound prompt, inbound update, inbound response", len(result.Events))
	}
	var sawPrompt, sawUpdate, sawResponse bool
	for i, event := range result.Events {
		if event.Seq != i+1 {
			t.Fatalf("event %d seq = %d, want %d", i, event.Seq, i+1)
		}
		if event.At.IsZero() {
			t.Fatalf("event %d At is zero", i)
		}
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			t.Fatalf("event %d invalid json-rpc message: %v\n%s", i, err, event.Message)
		}
		var msg struct {
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(event.Message, &msg); err != nil {
			t.Fatalf("unmarshal event %d: %v", i, err)
		}
		switch {
		case event.Direction == EventDirectionOutbound && msg.Method == "session/prompt":
			sawPrompt = true
		case event.Direction == EventDirectionInbound && msg.Method == "session/update":
			sawUpdate = true
		case event.Direction == EventDirectionInbound && len(msg.Result) > 0:
			sawResponse = true
		}
	}
	if !sawPrompt || !sawUpdate || !sawResponse {
		t.Fatalf("events missing expected envelopes: prompt=%t update=%t response=%t events=%#v", sawPrompt, sawUpdate, sawResponse, result.Events)
	}
}

func TestClientRunPromptInvokesSessionLifecycleHooks(t *testing.T) {
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

	var started string
	var cancelChecks atomic.Int64
	var badSession atomic.Value
	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		SessionStarted: func(sessionID string) error {
			started = sessionID
			return nil
		},
		CancelRequested: func(sessionID string) bool {
			if sessionID != "session-echo" {
				badSession.Store(sessionID)
			}
			cancelChecks.Add(1)
			return false
		},
	})

	if _, err := client.RunPrompt(t.Context(), "hello"); err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if started != "session-echo" {
		t.Fatalf("SessionStarted got %q, want session-echo", started)
	}
	if got := badSession.Load(); got != nil {
		t.Fatalf("CancelRequested sessionID = %q", got)
	}
	if cancelChecks.Load() == 0 {
		t.Fatal("CancelRequested was not checked")
	}
}

func TestClientSessionRunnerReusesSessionForMultiplePrompts(t *testing.T) {
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

	agent := &recordingAgent{newSessionID: "session-reuse"}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	var started string
	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		SessionStarted: func(sessionID string) error {
			started = sessionID
			return nil
		},
	})
	runner, err := client.StartSession(ctx, "")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if got, want := runner.SessionID(), acpsdk.SessionId("session-reuse"); got != want {
		t.Fatalf("runner SessionID = %q, want %q", got, want)
	}
	if started != "session-reuse" {
		t.Fatalf("SessionStarted = %q, want session-reuse", started)
	}

	first, err := runner.Prompt(ctx, "first")
	if err != nil {
		t.Fatalf("Prompt first: %v", err)
	}
	second, err := runner.Prompt(ctx, "second")
	if err != nil {
		t.Fatalf("Prompt second: %v", err)
	}
	if first.Text != "echo: first" || second.Text != "echo: second" {
		t.Fatalf("texts = %q/%q, want echo responses", first.Text, second.Text)
	}

	snap := agent.snapshot()
	if snap.initializeCount != 1 {
		t.Fatalf("initialize count = %d, want 1", snap.initializeCount)
	}
	if snap.newSessionCount != 1 {
		t.Fatalf("new session count = %d, want 1", snap.newSessionCount)
	}
	if got, want := strings.Join(snap.promptSessionIDs, ","), "session-reuse,session-reuse"; got != want {
		t.Fatalf("prompt session ids = %q, want %q", got, want)
	}
}

func TestClientSessionRunnerLoadsExistingSessionWhenSupported(t *testing.T) {
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

	agent := &recordingAgent{loadSupported: true, newSessionID: "new-session"}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	var started string
	cwd := t.TempDir()
	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     cwd,
		Timeout: 5 * time.Second,
		SessionStarted: func(sessionID string) error {
			started = sessionID
			return nil
		},
	})
	runner, err := client.StartSession(ctx, "existing-session")
	if err != nil {
		t.Fatalf("StartSession existing: %v", err)
	}
	if got, want := runner.SessionID(), acpsdk.SessionId("existing-session"); got != want {
		t.Fatalf("runner SessionID = %q, want %q", got, want)
	}
	if started != "" {
		t.Fatalf("SessionStarted on load = %q, want not called", started)
	}
	if _, err := runner.Prompt(ctx, "resume"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	snap := agent.snapshot()
	if snap.newSessionCount != 0 {
		t.Fatalf("new session count = %d, want 0", snap.newSessionCount)
	}
	if got, want := strings.Join(snap.loadSessionIDs, ","), "existing-session"; got != want {
		t.Fatalf("load session ids = %q, want %q", got, want)
	}
	if snap.loadCwd != cwd {
		t.Fatalf("load cwd = %q, want %q", snap.loadCwd, cwd)
	}
	if got, want := strings.Join(snap.promptSessionIDs, ","), "existing-session"; got != want {
		t.Fatalf("prompt session ids = %q, want %q", got, want)
	}
}

func TestClientSessionRunnerRejectsExistingSessionWhenLoadUnsupported(t *testing.T) {
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

	agent := &recordingAgent{loadSupported: false, newSessionID: "new-session"}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	})
	_, err := client.StartSession(ctx, "existing-session")
	if err == nil || !strings.Contains(err.Error(), "load existing acp session") {
		t.Fatalf("StartSession error = %v, want load unsupported error", err)
	}
	snap := agent.snapshot()
	if snap.newSessionCount != 0 {
		t.Fatalf("new session count = %d, want 0", snap.newSessionCount)
	}
	if len(snap.loadSessionIDs) != 0 {
		t.Fatalf("load session ids = %#v, want none", snap.loadSessionIDs)
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

type recordingAgent struct {
	conn          *acpsdk.AgentSideConnection
	loadSupported bool
	newSessionID  acpsdk.SessionId

	mu               sync.Mutex
	initializeCount  int
	newSessionCount  int
	loadSessionIDs   []string
	loadCwd          string
	promptSessionIDs []string
}

var _ acpsdk.Agent = (*recordingAgent)(nil)
var _ acpsdk.AgentLoader = (*recordingAgent)(nil)

func (*recordingAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (a *recordingAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	a.mu.Lock()
	a.initializeCount++
	a.mu.Unlock()
	return acpsdk.InitializeResponse{
		AgentInfo:         &acpsdk.Implementation{Name: "recording-agent", Version: "test"},
		AgentCapabilities: acpsdk.AgentCapabilities{LoadSession: a.loadSupported},
	}, nil
}

func (*recordingAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (a *recordingAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newSessionCount++
	return acpsdk.NewSessionResponse{SessionId: a.newSessionID}, nil
}

func (a *recordingAgent) LoadSession(_ context.Context, params acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadSessionIDs = append(a.loadSessionIDs, string(params.SessionId))
	a.loadCwd = params.Cwd
	return acpsdk.LoadSessionResponse{}, nil
}

func (a *recordingAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	a.mu.Lock()
	a.promptSessionIDs = append(a.promptSessionIDs, string(params.SessionId))
	a.mu.Unlock()
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

func (*recordingAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

type recordingAgentSnapshot struct {
	initializeCount  int
	newSessionCount  int
	loadSessionIDs   []string
	loadCwd          string
	promptSessionIDs []string
}

func (a *recordingAgent) snapshot() recordingAgentSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return recordingAgentSnapshot{
		initializeCount:  a.initializeCount,
		newSessionCount:  a.newSessionCount,
		loadSessionIDs:   append([]string(nil), a.loadSessionIDs...),
		loadCwd:          a.loadCwd,
		promptSessionIDs: append([]string(nil), a.promptSessionIDs...),
	}
}
