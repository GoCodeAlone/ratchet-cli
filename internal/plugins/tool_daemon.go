package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// daemonStopTimeout is the time to wait for a daemon process to exit cleanly.
const daemonStopTimeout = 5 * time.Second

// jsonrpcRequest is a JSON-RPC 2.0 request envelope.
type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response envelope.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message)
}

// initializeResult is the expected shape of the "initialize" response's result.
type initializeResult struct {
	Protocol string    `json:"protocol"`
	Tools    []ToolDef `json:"tools"`
}

// callParams is the params for a "call" request.
type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// DaemonTool manages a long-running plugin process that handles multiple tool
// calls over its lifetime using JSON-RPC 2.0 over stdin/stdout.
type DaemonTool struct {
	binPath  string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	mu       sync.Mutex
	nextID   atomic.Int64
	defs     []ToolDef
	waitOnce sync.Once
	waitErr  error
}

// StartDaemon launches the daemon binary, performs the initialize handshake,
// and returns a ready DaemonTool.
func StartDaemon(ctx context.Context, binPath string) (*DaemonTool, error) {
	cmd := exec.CommandContext(ctx, binPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start daemon %s: %w", binPath, err)
	}

	d := &DaemonTool{
		binPath: binPath,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdoutPipe),
	}

	if err := d.initialize(); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = d.wait() // reap zombie exactly once
		return nil, fmt.Errorf("initialize daemon: %w", err)
	}
	return d, nil
}

// initialize sends the JSON-RPC initialize request and reads the tool list.
func (d *DaemonTool) initialize() error {
	id := int(d.nextID.Add(1))
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
		Params:  map[string]any{},
	}
	resp, err := d.roundTrip(req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}

	var result initializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}
	d.defs = result.Tools
	return nil
}

// roundTrip sends a JSON-RPC request and reads a response. Caller must hold mu.
func (d *DaemonTool) roundTrip(req jsonrpcRequest) (*jsonrpcResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := d.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	line, err := d.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.ID != req.ID {
		return nil, fmt.Errorf("response ID mismatch: got %d want %d", resp.ID, req.ID)
	}
	return &resp, nil
}

// wait reaps the child process exactly once and caches the error.
func (d *DaemonTool) wait() error {
	d.waitOnce.Do(func() { d.waitErr = d.cmd.Wait() })
	return d.waitErr
}

// Call invokes a named tool on the daemon. It is goroutine-safe.
func (d *DaemonTool) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	id := int(d.nextID.Add(1))
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "call",
		Params:  callParams{Name: name, Arguments: args},
	}

	// Honour context cancellation while holding the lock.
	type result struct {
		resp *jsonrpcResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := d.roundTrip(req)
		ch <- result{resp, err}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled while I/O is in flight. The goroutine is blocked
		// on stdin/stdout and will corrupt the JSON-RPC stream for the next call.
		// Kill the daemon to prevent interleaved responses, then reap via wait().
		// wait() uses sync.Once so it's safe even if Stop() is called concurrently.
		_ = d.cmd.Process.Kill()
		_ = d.wait()
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		if r.resp.Error != nil {
			return nil, r.resp.Error
		}
		var val any
		if err := json.Unmarshal(r.resp.Result, &val); err != nil {
			return nil, fmt.Errorf("parse call result: %w", err)
		}
		return val, nil
	}
}

// Defs returns the tool definitions declared by the daemon at initialize time.
func (d *DaemonTool) Defs() []ToolDef { return d.defs }

// Stop gracefully shuts down the daemon: closes stdin, waits for exit with a
// timeout, then kills if necessary.
func (d *DaemonTool) Stop() error {
	_ = d.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- d.wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(daemonStopTimeout):
		_ = d.cmd.Process.Kill()
		return <-done
	}
}

// ---------------------------------------------------------------------------
// DaemonToolRef — implements plugin.Tool for a single tool within a daemon
// ---------------------------------------------------------------------------

// DaemonToolRef wraps a single named tool exposed by a DaemonTool.
type DaemonToolRef struct {
	daemon *DaemonTool
	def    ToolDef
}

// NewDaemonToolRef creates a DaemonToolRef for the named tool.
func NewDaemonToolRef(daemon *DaemonTool, def ToolDef) *DaemonToolRef {
	return &DaemonToolRef{daemon: daemon, def: def}
}

// Name implements plugin.Tool.
func (r *DaemonToolRef) Name() string { return r.def.Name }

// Description implements plugin.Tool.
func (r *DaemonToolRef) Description() string { return r.def.Description }

// Definition implements plugin.Tool.
func (r *DaemonToolRef) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        r.def.Name,
		Description: r.def.Description,
		Parameters:  r.def.Parameters,
	}
}

// Execute implements plugin.Tool by delegating to the parent daemon.
func (r *DaemonToolRef) Execute(ctx context.Context, args map[string]any) (any, error) {
	return r.daemon.Call(ctx, r.def.Name, args)
}
