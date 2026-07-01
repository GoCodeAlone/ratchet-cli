package acpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

const defaultTimeout = 30 * time.Second

type Client struct {
	conn      *acpsdk.ClientSideConnection
	callbacks *Callbacks
	timeout   time.Duration
	cmd       *exec.Cmd
	stderr    *lockedBuffer
	wait      chan error
}

type Result struct {
	SessionID  acpsdk.SessionId
	StopReason acpsdk.StopReason
	Updates    []acpsdk.SessionNotification
	Text       string
	Stderr     string
	Duration   time.Duration
}

func NewInProcessClient(peerInput io.Writer, peerOutput io.Reader, opts RunOptions) *Client {
	callbacks := NewCallbacks(opts)
	return &Client{
		conn:      acpsdk.NewClientSideConnection(callbacks, peerInput, peerOutput),
		callbacks: callbacks,
		timeout:   timeoutOrDefault(opts.Timeout),
	}
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
	}
	go func() {
		client.wait <- cmd.Wait()
	}()
	return client, nil
}

func (c *Client) RunPrompt(ctx context.Context, prompt string) (Result, error) {
	started := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	c.callbacks.Reset()

	if _, err := c.conn.Initialize(callCtx, acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpsdk.ClientCapabilities{
			Fs: acpsdk.FileSystemCapability{
				ReadTextFile:  true,
				WriteTextFile: c.callbacks.allowWrites,
			},
		},
	}); err != nil {
		return Result{}, fmt.Errorf("initialize acp agent: %w", err)
	}
	session, err := c.conn.NewSession(callCtx, acpsdk.NewSessionRequest{
		Cwd:        c.callbacks.Cwd(),
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		return Result{}, fmt.Errorf("create acp session: %w", err)
	}
	updateCount := c.callbacks.UpdateCount()
	resp, err := c.conn.Prompt(callCtx, acpsdk.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(prompt)},
	})
	if err != nil {
		return Result{}, fmt.Errorf("send acp prompt: %w", err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, min(c.timeout, 500*time.Millisecond))
	defer waitCancel()
	c.callbacks.WaitForUpdate(waitCtx, updateCount)
	updates, text := c.callbacks.Snapshot()
	return Result{
		SessionID:  session.SessionId,
		StopReason: resp.StopReason,
		Updates:    updates,
		Text:       text,
		Stderr:     c.stderrString(),
		Duration:   time.Since(started),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	if c.wait == nil {
		return nil
	}
	select {
	case err := <-c.wait:
		return err
	default:
	}
	_ = c.cmd.Process.Kill()
	select {
	case <-c.wait:
		return nil
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

func (c *Client) stderrString() string {
	if c == nil || c.stderr == nil {
		return ""
	}
	return c.stderr.String()
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
