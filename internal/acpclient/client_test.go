package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
		CancelRequested: func(sessionID string) (bool, error) {
			if sessionID != "session-echo" {
				badSession.Store(sessionID)
			}
			cancelChecks.Add(1)
			return false, nil
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

func TestClientRunPromptReturnsCancellationAuthorityError(t *testing.T) {
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
	authorityErr := errors.New("cancellation authority unavailable")
	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
		CancelRequested: func(string) (bool, error) {
			return false, authorityErr
		},
	})

	_, err := client.RunPrompt(t.Context(), "hello")
	if !errors.Is(err, authorityErr) {
		t.Fatalf("RunPrompt error = %v, want authority error", err)
	}
}

func TestClientCancellationAuthorityErrorIncludesTeardownFailure(t *testing.T) {
	authorityErr := errors.New("cancellation authority unavailable")
	teardownErr := errors.New("close cancellation transport")
	client := &Client{
		cancelReq: func(string) (bool, error) {
			return false, authorityErr
		},
		transports: []io.Closer{closeFunc(func() error {
			return teardownErr
		})},
	}
	executionCtx, cancelExecution := context.WithCancelCause(t.Context())

	err := client.startCancelWatcher(t.Context(), "authority-session", cancelExecution)()
	if !errors.Is(err, authorityErr) {
		t.Fatalf("cancel watcher error = %v, want authority error", err)
	}
	if !errors.Is(err, teardownErr) {
		t.Fatalf("cancel watcher error = %v, want teardown error", err)
	}
	if got := context.Cause(executionCtx); !errors.Is(got, authorityErr) {
		t.Fatalf("execution cause = %v, want authority error", got)
	}
	if strings.Index(err.Error(), authorityErr.Error()) > strings.Index(err.Error(), teardownErr.Error()) {
		t.Fatalf("cancel watcher error = %q, want authority error first", err)
	}
}

func TestClientCancellationCauseWinsRacingPromptResponse(t *testing.T) {
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentR.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientR.Close()
		_ = agentToClientW.Close()
	})

	agent := &cancellationRaceAgent{
		promptStarted: make(chan struct{}),
		releasePrompt: make(chan struct{}),
		cancelCalled:  make(chan struct{}),
	}
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	requested := &atomic.Bool{}
	client := NewInProcessClient(clientToAgentW, agentToClientR, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 2 * time.Second,
		CancelRequested: func(string) (bool, error) {
			return requested.Load(), nil
		},
	})

	done := make(chan error, 1)
	go func() {
		_, err := client.RunPrompt(t.Context(), "race cancellation")
		done <- err
	}()
	<-agent.promptStarted
	requested.Store(true)
	select {
	case <-agent.cancelCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("ACP cancel was not sent")
	}
	close(agent.releasePrompt)
	if err := <-done; !errors.Is(err, ErrCancelRequested) {
		t.Fatalf("RunPrompt error = %v, want ErrCancelRequested", err)
	}
	if got := agent.cancelCount.Load(); got != 1 {
		t.Fatalf("ACP cancel count = %d, want 1", got)
	}
	select {
	case <-agentConn.Done():
	default:
	}
}

func TestClientCancellationIgnoringAgentReturnsAndReapsProcess(t *testing.T) {
	requested := &atomic.Bool{}
	client, cancelMarker, promptMarker := startCancellationProcessClient(t, requested, nil)
	runner, err := client.StartSession(t.Context(), "")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := runner.Prompt(t.Context(), "ignore cancellation")
		done <- err
	}()
	waitForCancellationMarker(t, promptMarker)
	requested.Store(true)
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelRequested) {
			t.Fatalf("Prompt error = %v, want ErrCancelRequested", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Prompt did not return after cancellation")
	}
	assertCancellationProcessReaped(t, client)
	if got := markerLineCount(t, cancelMarker); got != 1 {
		t.Fatalf("ACP cancel count = %d, want 1", got)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
}

func TestClientCancellationAuthorityErrorsKillWithoutACP(t *testing.T) {
	for _, requestedWithError := range []bool{false, true} {
		t.Run(fmt.Sprintf("requested_%t", requestedWithError), func(t *testing.T) {
			authorityErr := errors.New("cancellation authority unavailable")
			requested := &atomic.Bool{}
			client, cancelMarker, promptMarker := startCancellationProcessClient(t, requested, func(string) (bool, error) {
				if !requested.Load() {
					return false, nil
				}
				return requestedWithError, authorityErr
			})
			runner, err := client.StartSession(t.Context(), "")
			if err != nil {
				t.Fatalf("StartSession: %v", err)
			}

			done := make(chan error, 1)
			go func() {
				_, err := runner.Prompt(t.Context(), "authority failure")
				done <- err
			}()
			waitForCancellationMarker(t, promptMarker)
			requested.Store(true)
			select {
			case err := <-done:
				if !errors.Is(err, authorityErr) {
					t.Fatalf("Prompt error = %v, want authority error", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Prompt did not return after authority error")
			}
			assertCancellationProcessReaped(t, client)
			if got := markerLineCount(t, cancelMarker); got != 0 {
				t.Fatalf("ACP cancel count = %d, want 0", got)
			}
		})
	}
}

func TestClientCancellationBlockedSendReturnsAndReapsProcess(t *testing.T) {
	processExited := make(chan struct{})
	blocker := &blockingCancelWriter{started: make(chan struct{}), processExited: processExited}
	requested := &atomic.Bool{}
	client, cancelMarker, promptMarker := startCancellationProcessClientWithWriter(t, requested, blocker, processExited)
	runner, err := client.StartSession(t.Context(), "")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := runner.Prompt(t.Context(), "blocked cancel send")
		done <- err
	}()
	waitForCancellationMarker(t, promptMarker)
	requested.Store(true)
	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("ACP cancel send did not block")
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelRequested) {
			t.Fatalf("Prompt error = %v, want ErrCancelRequested", err)
		}
	case <-time.After(3 * time.Second):
		_ = client.Close()
		t.Fatal("Prompt did not return after blocked cancel send")
	}
	assertCancellationProcessReaped(t, client)
	if got := markerLineCount(t, cancelMarker); got != 0 {
		t.Fatalf("delivered ACP cancel count = %d, want 0 for blocked send", got)
	}
}

func TestClientCancellationSendJoinIsBounded(t *testing.T) {
	writer := &stubbornCancelWriter{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	peerOutputR, peerOutputW := io.Pipe()
	t.Cleanup(func() { _ = peerOutputW.Close() })
	defer func() {
		close(writer.release)
		select {
		case <-writer.finished:
		case <-time.After(time.Second):
			t.Fatal("stubborn cancel writer did not finish after release")
		}
	}()
	client := NewInProcessClient(writer, peerOutputR, RunOptions{
		Timeout: 50 * time.Millisecond,
		CancelRequested: func(string) (bool, error) {
			return true, nil
		},
	})
	executionCtx, cancelExecution := context.WithCancelCause(t.Context())
	watcherReady := make(chan func() error, 1)
	go func() {
		watcherReady <- client.startCancelWatcher(t.Context(), "blocked-send-session", cancelExecution)
	}()

	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("ACP cancel send did not start")
	}
	select {
	case stopWatcher := <-watcherReady:
		err := stopWatcher()
		if !errors.Is(err, ErrCancelRequested) {
			t.Fatalf("cancel watcher error = %v, want ErrCancelRequested", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancel watcher error = %v, want bounded join deadline", err)
		}
		if got := context.Cause(executionCtx); !errors.Is(got, ErrCancelRequested) {
			t.Fatalf("execution cause = %v, want ErrCancelRequested", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel watcher did not bound the blocked send join")
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

const (
	cancellationProcessHelperEnv = "RATCHET_ACP_CANCELLATION_PROCESS_HELPER"
	cancellationPromptMarkerEnv  = "RATCHET_ACP_CANCELLATION_PROMPT_MARKER"
	cancellationCancelMarkerEnv  = "RATCHET_ACP_CANCELLATION_CANCEL_MARKER"
)

func TestClientCancellationProcessHelper(t *testing.T) {
	if os.Getenv(cancellationProcessHelperEnv) != "1" {
		return
	}
	agent := &ignoringCancellationAgent{
		promptMarker: os.Getenv(cancellationPromptMarkerEnv),
		cancelMarker: os.Getenv(cancellationCancelMarkerEnv),
	}
	conn := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	<-conn.Done()
}

func startCancellationProcessClient(t *testing.T, requested *atomic.Bool, callback func(string) (bool, error)) (*Client, string, string) {
	t.Helper()
	cancelMarker, promptMarker := configureCancellationProcess(t)
	if callback == nil {
		callback = func(string) (bool, error) {
			return requested.Load(), nil
		}
	}
	client, err := Start(t.Context(), cancellationProcessSpec(), RunOptions{
		Cwd:             t.TempDir(),
		Timeout:         time.Second,
		CancelRequested: callback,
	})
	if err != nil {
		t.Fatalf("Start cancellation process: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, cancelMarker, promptMarker
}

func startCancellationProcessClientWithWriter(t *testing.T, requested *atomic.Bool, blocker *blockingCancelWriter, processExited chan struct{}) (*Client, string, string) {
	t.Helper()
	cancelMarker, promptMarker := configureCancellationProcess(t)
	cmd := exec.CommandContext(t.Context(), cancellationProcessSpec().Command, cancellationProcessSpec().Args...)
	cmd.Dir = t.TempDir()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	blocker.target = stdin
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cancellation process: %v", err)
	}
	callbacks := NewCallbacks(RunOptions{Cwd: cmd.Dir})
	client := &Client{
		conn:      acpsdk.NewClientSideConnection(callbacks, blocker, stdout),
		callbacks: callbacks,
		timeout:   time.Second,
		cmd:       cmd,
		stderr:    stderr,
		wait:      make(chan error, 1),
		cancelReq: func(string) (bool, error) { return requested.Load(), nil },
	}
	go func() {
		err := cmd.Wait()
		close(processExited)
		client.wait <- err
	}()
	t.Cleanup(func() {
		_ = client.Close()
		_ = blocker.Close()
	})
	return client, cancelMarker, promptMarker
}

func configureCancellationProcess(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cancelMarker := filepath.Join(dir, "cancel.log")
	promptMarker := filepath.Join(dir, "prompt.started")
	t.Setenv(cancellationProcessHelperEnv, "1")
	t.Setenv(cancellationPromptMarkerEnv, promptMarker)
	t.Setenv(cancellationCancelMarkerEnv, cancelMarker)
	return cancelMarker, promptMarker
}

func cancellationProcessSpec() AgentSpec {
	return AgentSpec{
		Name:    "cancellation-helper",
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestClientCancellationProcessHelper$"},
	}
}

func waitForCancellationMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat cancellation marker: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}

func markerLineCount(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return len(strings.Fields(string(b)))
}

func assertCancellationProcessReaped(t *testing.T, client *Client) {
	t.Helper()
	if client.cmd == nil || client.cmd.ProcessState == nil {
		t.Fatalf("cancellation process was not reaped: %#v", client.cmd.ProcessState)
	}
}

type blockingCancelWriter struct {
	target        io.WriteCloser
	started       chan struct{}
	processExited <-chan struct{}
	once          sync.Once
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

type stubbornCancelWriter struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	once     sync.Once
}

func (w *stubbornCancelWriter) Write([]byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	close(w.finished)
	return 0, io.ErrClosedPipe
}

func (*stubbornCancelWriter) Close() error {
	return nil
}

func (w *blockingCancelWriter) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte(`"session/cancel"`)) {
		return w.target.Write(p)
	}
	w.once.Do(func() { close(w.started) })
	<-w.processExited
	return 0, io.ErrClosedPipe
}

func (w *blockingCancelWriter) Close() error {
	if w == nil || w.target == nil {
		return nil
	}
	return w.target.Close()
}

type ignoringCancellationAgent struct {
	promptMarker string
	cancelMarker string
}

var _ acpsdk.Agent = (*ignoringCancellationAgent)(nil)

func (*ignoringCancellationAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*ignoringCancellationAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{AgentInfo: &acpsdk.Implementation{Name: "ignoring-agent", Version: "test"}}, nil
}

func (a *ignoringCancellationAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	f, err := os.OpenFile(a.cancelMarker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("cancel\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (*ignoringCancellationAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "cancellation-session"}, nil
}

func (a *ignoringCancellationAgent) Prompt(context.Context, acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	if err := os.WriteFile(a.promptMarker, []byte("started\n"), 0o600); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	select {}
}

func (*ignoringCancellationAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

type cancellationRaceAgent struct {
	promptStarted chan struct{}
	releasePrompt chan struct{}
	cancelCalled  chan struct{}
	cancelOnce    sync.Once
	cancelCount   atomic.Int64
}

var _ acpsdk.Agent = (*cancellationRaceAgent)(nil)

func (*cancellationRaceAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*cancellationRaceAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{AgentInfo: &acpsdk.Implementation{Name: "cancel-race-agent", Version: "test"}}, nil
}

func (a *cancellationRaceAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	a.cancelCount.Add(1)
	a.cancelOnce.Do(func() { close(a.cancelCalled) })
	return nil
}

func (*cancellationRaceAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "cancel-race-session"}, nil
}

func (a *cancellationRaceAgent) Prompt(context.Context, acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	close(a.promptStarted)
	<-a.releasePrompt
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*cancellationRaceAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
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
