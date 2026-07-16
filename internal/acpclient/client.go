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
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

const defaultTimeout = 30 * time.Second

var ErrCancellationTransportUnsupported = errors.New("acp client cancellation requires closable in-process transports")

type Client struct {
	conn               *acpsdk.ClientSideConnection
	callbacks          *Callbacks
	timeout            time.Duration
	setupErr           error
	cmd                *exec.Cmd
	killProcess        func() error
	stderr             *lockedBuffer
	wait               chan error
	onSession          func(string) error
	cancelReq          func(string) (bool, error)
	cancelPollInterval time.Duration
	transports         []io.Closer
	closeOnce          sync.Once
	closeErr           error
	transportOnce      sync.Once
	transportError     error
}

type Result struct {
	SessionID  acpsdk.SessionId
	StopReason acpsdk.StopReason
	Updates    []acpsdk.SessionNotification
	Events     []EventLogLine
	Text       string
	Stderr     string
	Duration   time.Duration
}

type SessionRunner struct {
	client    *Client
	sessionID acpsdk.SessionId
}

func NewInProcessClient(peerInput io.Writer, peerOutput io.Reader, opts RunOptions) *Client {
	callbacks := NewCallbacks(opts)
	client := &Client{
		callbacks: callbacks,
		timeout:   timeoutOrDefault(opts.Timeout),
		onSession: opts.SessionStarted,
		cancelReq: opts.CancelRequested,
	}
	if opts.CancelRequested != nil {
		_, inputClosable := peerInput.(io.Closer)
		_, outputClosable := peerOutput.(io.Closer)
		if !inputClosable || !outputClosable {
			client.setupErr = ErrCancellationTransportUnsupported
			return client
		}
	}
	client.conn = acpsdk.NewClientSideConnection(callbacks, peerInput, peerOutput)
	client.addTransport(peerInput)
	client.addTransport(peerOutput)
	return client
}

func Start(ctx context.Context, spec AgentSpec, opts RunOptions) (*Client, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open acp agent stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open acp agent stdout: %w", err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start acp agent %q: %w", spec.Command, err)
	}
	callbacks := NewCallbacks(opts)
	client := &Client{
		conn:      acpsdk.NewClientSideConnection(callbacks, stdin, stdout),
		callbacks: callbacks,
		timeout:   timeoutOrDefault(opts.Timeout),
		cmd:       cmd,
		stderr:    stderr,
		wait:      make(chan error, 1),
		onSession: opts.SessionStarted,
		cancelReq: opts.CancelRequested,
		transports: []io.Closer{
			stdin,
			stdout,
		},
	}
	go func() {
		client.wait <- cmd.Wait()
	}()
	return client, nil
}

func (c *Client) RunPrompt(ctx context.Context, prompt string) (Result, error) {
	runner, err := c.StartSession(ctx, "")
	if err != nil {
		return Result{}, err
	}
	return runner.Prompt(ctx, prompt)
}

func (c *Client) StartSession(ctx context.Context, existingSessionID string) (*SessionRunner, error) {
	if c.setupErr != nil {
		return nil, c.setupErr
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	initResp, err := c.conn.Initialize(callCtx, acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpsdk.ClientCapabilities{
			Fs: acpsdk.FileSystemCapability{
				ReadTextFile:  true,
				WriteTextFile: c.callbacks.allowWrites,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize acp agent: %w", err)
	}
	existingSessionID = strings.TrimSpace(existingSessionID)
	if existingSessionID != "" {
		if !initResp.AgentCapabilities.LoadSession {
			return nil, fmt.Errorf("load existing acp session %s: agent does not support loadSession", existingSessionID)
		}
		sessionID := acpsdk.SessionId(existingSessionID)
		if _, err := c.conn.LoadSession(callCtx, acpsdk.LoadSessionRequest{
			Cwd:        c.callbacks.Cwd(),
			McpServers: []acpsdk.McpServer{},
			SessionId:  sessionID,
		}); err != nil {
			return nil, fmt.Errorf("load existing acp session %s: %w", existingSessionID, err)
		}
		return &SessionRunner{client: c, sessionID: sessionID}, nil
	}
	session, err := c.conn.NewSession(callCtx, acpsdk.NewSessionRequest{
		Cwd:        c.callbacks.Cwd(),
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		return nil, fmt.Errorf("create acp session: %w", err)
	}
	if c.onSession != nil {
		if err := c.onSession(string(session.SessionId)); err != nil {
			return nil, fmt.Errorf("record acp session: %w", err)
		}
	}
	return &SessionRunner{client: c, sessionID: session.SessionId}, nil
}

func (r *SessionRunner) SessionID() acpsdk.SessionId {
	if r == nil {
		return ""
	}
	return r.sessionID
}

func (r *SessionRunner) LastEvents() []EventLogLine {
	if r == nil || r.client == nil || r.client.callbacks == nil {
		return nil
	}
	return r.client.callbacks.EventSnapshot()
}

func (r *SessionRunner) Prompt(ctx context.Context, prompt string) (Result, error) {
	if r == nil || r.client == nil {
		return Result{}, errors.New("acp session runner is not initialized")
	}
	started := time.Now()
	c := r.client
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	c.callbacks.Reset()
	requestID := fmt.Sprintf("ratchet-prompt-%d", started.UnixNano())
	if err := c.callbacks.RecordEvent(EventDirectionOutbound, promptRequestEventMessage(requestID, r.sessionID, prompt)); err != nil {
		return Result{}, err
	}
	stopCancelWatcher, err := c.startCancelWatcher(callCtx, r.sessionID)
	if err != nil {
		return Result{}, err
	}
	updateCount := c.callbacks.UpdateCount()
	// The SDK auto-sends an unbounded cancel when its prompt context is canceled.
	// Keep that context live so the causal watcher owns the single bounded send.
	resp, err := c.conn.Prompt(callCtx, acpsdk.PromptRequest{
		SessionId: r.sessionID,
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(prompt)},
	})
	if cancelErr := stopCancelWatcher(); cancelErr != nil {
		return Result{}, cancelErr
	}
	if err != nil {
		_ = c.callbacks.RecordEvent(EventDirectionInbound, promptErrorEventMessage(requestID, err))
		return Result{}, fmt.Errorf("send acp prompt: %w", err)
	}
	if err := c.callbacks.RecordEvent(EventDirectionInbound, promptResponseEventMessage(requestID, resp)); err != nil {
		return Result{}, err
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, min(c.timeout, 500*time.Millisecond))
	defer waitCancel()
	c.callbacks.WaitForUpdate(waitCtx, updateCount)
	updates, text := c.callbacks.Snapshot()
	events := c.callbacks.EventSnapshot()
	return Result{
		SessionID:  r.sessionID,
		StopReason: resp.StopReason,
		Updates:    updates,
		Events:     events,
		Text:       text,
		Stderr:     c.stderrString(),
		Duration:   time.Since(started),
	}, nil
}

func promptRequestEventMessage(id string, sessionID acpsdk.SessionId, prompt string) json.RawMessage {
	return mustJSONRaw(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": string(sessionID),
			"prompt": []map[string]any{{
				"type": "text",
				"text": prompt,
			}},
		},
	})
}

func sessionUpdateEventMessage(n acpsdk.SessionNotification) json.RawMessage {
	return mustJSONRaw(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": string(n.SessionId),
			"update":    n.Update,
		},
	})
}

func promptResponseEventMessage(id string, resp acpsdk.PromptResponse) json.RawMessage {
	return mustJSONRaw(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"stopReason": resp.StopReason,
		},
	})
}

func promptErrorEventMessage(id string, err error) json.RawMessage {
	return mustJSONRaw(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32000,
			"message": err.Error(),
		},
	})
}

func mustJSONRaw(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return b
}

func (c *Client) Close() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.wait == nil {
			return
		}
		select {
		case c.closeErr = <-c.wait:
			return
		default:
		}
		kill := c.cmd.Process.Kill
		if c.killProcess != nil {
			kill = c.killProcess
		}
		if err := kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			c.closeErr = fmt.Errorf("kill acp agent: %w", err)
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-c.wait:
		case <-timer.C:
			c.closeErr = errors.Join(c.closeErr, context.DeadlineExceeded)
		}
	})
	return c.closeErr
}

func (c *Client) stderrString() string {
	if c == nil || c.stderr == nil {
		return ""
	}
	return c.stderr.String()
}

func (c *Client) startCancelWatcher(ctx context.Context, sessionID acpsdk.SessionId) (func() error, error) {
	if c == nil || c.cancelReq == nil {
		return func() error { return nil }, nil
	}
	done := make(chan struct{})
	finished := make(chan error, 1)
	var checkMu sync.Mutex
	var latchedErr error
	check := func() error {
		checkMu.Lock()
		defer checkMu.Unlock()
		if latchedErr != nil {
			return latchedErr
		}
		requested, err := c.cancelReq(string(sessionID))
		if err != nil {
			latchedErr = errors.Join(err, c.terminateCancellation())
			return latchedErr
		}
		if !requested {
			return nil
		}
		latchedErr = errors.Join(ErrCancelRequested, c.sendCancelAndTerminate(ctx, sessionID))
		return latchedErr
	}
	if err := check(); err != nil {
		return nil, err
	}
	go func() {
		interval := c.cancelPollInterval
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				finished <- nil
				return
			case <-ctx.Done():
				finished <- nil
				return
			case <-ticker.C:
				if err := check(); err != nil {
					finished <- err
					return
				}
			}
		}
	}()
	return func() error {
		close(done)
		if err := <-finished; err != nil {
			return err
		}
		return check()
	}, nil
}

func (c *Client) sendCancelAndTerminate(ctx context.Context, sessionID acpsdk.SessionId) error {
	timeout := min(c.timeout, 5*time.Second)
	sendCtx, sendCancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer sendCancel()
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- c.conn.Cancel(sendCtx, acpsdk.CancelNotification{SessionId: sessionID})
	}()
	select {
	case sendErr := <-sendDone:
		return errors.Join(sendErr, c.terminateCancellation())
	case <-sendCtx.Done():
		sendCancel()
		teardownErr := c.terminateCancellation()
		sendErr := <-sendDone
		return errors.Join(context.Cause(sendCtx), sendErr, teardownErr)
	}
}

func (c *Client) terminateCancellation() error {
	if c == nil {
		return nil
	}
	processErr := c.Close()
	c.transportOnce.Do(func() {
		for _, transport := range c.transports {
			c.transportError = errors.Join(c.transportError, transport.Close())
		}
	})
	return errors.Join(processErr, c.transportError)
}

func (c *Client) addTransport(value any) {
	transport, ok := value.(io.Closer)
	if !ok {
		return
	}
	c.transports = append(c.transports, transport)
}

func timeoutOrDefault(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultTimeout
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
