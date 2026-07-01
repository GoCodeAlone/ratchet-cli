package acpclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	acpsdk "github.com/coder/acp-go-sdk"
)

var (
	ErrPathOutsideCWD = errors.New("acp client path is outside cwd")
	ErrWritesDisabled = errors.New("acp client filesystem writes are disabled")
)

type Callbacks struct {
	cwd         string
	realCwd     string
	allowWrites bool

	mu      sync.Mutex
	updates []acpsdk.SessionNotification
	text    strings.Builder
	notify  chan struct{}
}

var _ acpsdk.Client = (*Callbacks)(nil)

func NewCallbacks(opts RunOptions) *Callbacks {
	cwd := opts.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	cwd = filepath.Clean(cwd)
	realCwd := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		realCwd = filepath.Clean(resolved)
	}
	return &Callbacks{cwd: cwd, realCwd: realCwd, allowWrites: opts.AllowWrites, notify: make(chan struct{}, 1)}
}

func (c *Callbacks) Cwd() string {
	return c.cwd
}

func (c *Callbacks) SessionUpdate(ctx context.Context, n acpsdk.SessionNotification) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, n)
	if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
		c.text.WriteString(n.Update.AgentMessageChunk.Content.Text.Text)
	}
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return nil
}

func (c *Callbacks) Snapshot() ([]acpsdk.SessionNotification, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.updates), c.text.String()
}

func (c *Callbacks) Reset() {
	c.mu.Lock()
	c.updates = nil
	c.text.Reset()
	c.mu.Unlock()
	for {
		select {
		case <-c.notify:
		default:
			return
		}
	}
}

func (c *Callbacks) UpdateCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.updates)
}

func (c *Callbacks) WaitForUpdate(ctx context.Context, previous int) {
	for {
		c.mu.Lock()
		current := len(c.updates)
		c.mu.Unlock()
		if current > previous {
			return
		}
		select {
		case <-c.notify:
		case <-ctx.Done():
			return
		}
	}
}

func (c *Callbacks) ReadTextFile(_ context.Context, p acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	path, err := c.resolvePath(p.Path, true)
	if err != nil {
		return acpsdk.ReadTextFileResponse{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return acpsdk.ReadTextFileResponse{}, err
	}
	text := string(content)
	if p.Line != nil || p.Limit != nil {
		text = sliceLines(text, p.Line, p.Limit)
	}
	return acpsdk.ReadTextFileResponse{Content: text}, nil
}

func (c *Callbacks) WriteTextFile(_ context.Context, p acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	if !c.allowWrites {
		return acpsdk.WriteTextFileResponse{}, ErrWritesDisabled
	}
	path, err := c.resolvePath(p.Path, false)
	if err != nil {
		return acpsdk.WriteTextFileResponse{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return acpsdk.WriteTextFileResponse{}, err
	}
	return acpsdk.WriteTextFileResponse{}, os.WriteFile(path, []byte(p.Content), 0o644)
}

func (*Callbacks) RequestPermission(_ context.Context, p acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	for _, opt := range p.Options {
		if opt.Kind == acpsdk.PermissionOptionKindRejectOnce || opt.Kind == acpsdk.PermissionOptionKindRejectAlways {
			return acpsdk.RequestPermissionResponse{
				Outcome: acpsdk.NewRequestPermissionOutcomeSelected(opt.OptionId),
			}, nil
		}
	}
	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.NewRequestPermissionOutcomeCancelled(),
	}, nil
}

func (*Callbacks) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, errors.New("terminal callbacks are not supported in headless acp client mode")
}

func (*Callbacks) KillTerminalCommand(context.Context, acpsdk.KillTerminalCommandRequest) (acpsdk.KillTerminalCommandResponse, error) {
	return acpsdk.KillTerminalCommandResponse{}, nil
}

func (*Callbacks) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{Output: "terminal unsupported"}, nil
}

func (*Callbacks) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, nil
}

func (*Callbacks) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, nil
}

func (c *Callbacks) resolvePath(path string, mustExist bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.cwd, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(c.cwd, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideCWD, abs)
	}
	if err := c.validateRealPath(abs, mustExist); err != nil {
		return "", err
	}
	return abs, nil
}

func (c *Callbacks) validateRealPath(path string, mustExist bool) error {
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		return c.requireWithinRealCWD(filepath.Clean(realPath))
	}
	if mustExist {
		return err
	}

	parent := filepath.Dir(path)
	for {
		if parent == "." || parent == string(filepath.Separator) || parent == filepath.Dir(parent) {
			return err
		}
		if realParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			return c.requireWithinRealCWD(filepath.Clean(realParent))
		}
		parent = filepath.Dir(parent)
	}
}

func (c *Callbacks) requireWithinRealCWD(path string) error {
	rel, err := filepath.Rel(c.realCwd, path)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: %s", ErrPathOutsideCWD, path)
	}
	return nil
}

func sliceLines(text string, line *int, limit *int) string {
	lines := strings.Split(text, "\n")
	start := 0
	if line != nil && *line > 1 {
		start = min(*line-1, len(lines))
	}
	end := len(lines)
	if limit != nil && *limit >= 0 {
		end = min(start+*limit, end)
	}
	return strings.Join(lines[start:end], "\n")
}
