package acpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

var (
	ErrMissingCommand = errors.New("acp client command is required")
	ErrShellCommand   = errors.New("acp client command must be an executable path/name, with args passed separately")
	ErrUnknownAgent   = errors.New("unknown acp client agent")
)

type AgentSpec struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	EnvKeys []string `json:"envKeys,omitempty"`
}

type RunOptions struct {
	Agent           string
	Command         string
	Args            []string
	Cwd             string
	AllowWrites     bool
	Timeout         time.Duration
	SessionStarted  func(sessionID string) error
	CancelRequested func(sessionID string) bool
}

type SessionRecord struct {
	ID                 string         `json:"id"`
	Agent              string         `json:"agent"`
	CommandFingerprint string         `json:"commandFingerprint"`
	Cwd                string         `json:"cwd"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	LastStopReason     string         `json:"lastStopReason,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	Turns              []TurnSummary  `json:"turns,omitempty"`
	PendingPrompt      *PendingPrompt `json:"pendingPrompt,omitempty"`
}

const (
	SessionStatusQueued          = "queued"
	SessionStatusRunning         = "running"
	SessionStatusCompleted       = "completed"
	SessionStatusCanceled        = "canceled"
	SessionStatusCancelRequested = "cancel_requested"

	PendingPromptStatusPending  = "pending"
	PendingPromptStatusCanceled = "canceled"
)

type TurnSummary struct {
	Prompt     string    `json:"prompt"`
	Response   string    `json:"response"`
	StopReason string    `json:"stopReason"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PendingPrompt struct {
	ID         string     `json:"id"`
	Prompt     string     `json:"prompt"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	CanceledAt *time.Time `json:"canceledAt,omitempty"`
}

type OwnerLock struct {
	SessionID          string    `json:"sessionId"`
	PID                int       `json:"pid"`
	CommandFingerprint string    `json:"commandFingerprint"`
	StartedAt          time.Time `json:"startedAt"`
}

type CancelRequest struct {
	SessionID   string    `json:"sessionId"`
	RequestedAt time.Time `json:"requestedAt"`
}

type Registry struct {
	specs map[string]AgentSpec
}

func DefaultRegistry() Registry {
	return NewRegistry([]AgentSpec{
		{Name: "ratchet", Command: "ratchet", Args: []string{"acp"}},
		{Name: "codex", Command: "codex", Args: []string{"acp"}},
		{Name: "claude", Command: "claude", Args: []string{"acp"}},
		{Name: "gemini", Command: "gemini", Args: []string{"acp"}},
		{Name: "opencode", Command: "opencode", Args: []string{"acp"}},
		{Name: "custom"},
	})
}

func NewRegistry(specs []AgentSpec) Registry {
	reg := Registry{specs: make(map[string]AgentSpec, len(specs))}
	for _, spec := range specs {
		if spec.Name == "" {
			continue
		}
		reg.specs[spec.Name] = cloneSpec(spec)
	}
	return reg
}

func (r Registry) Lookup(name string) (AgentSpec, bool) {
	spec, ok := r.specs[name]
	if !ok {
		return AgentSpec{}, false
	}
	return cloneSpec(spec), true
}

func (r Registry) Resolve(opts RunOptions) (AgentSpec, error) {
	var spec AgentSpec
	if opts.Agent != "" {
		var ok bool
		spec, ok = r.Lookup(opts.Agent)
		if !ok {
			return AgentSpec{}, fmt.Errorf("%w: %s", ErrUnknownAgent, opts.Agent)
		}
	}
	if opts.Command != "" {
		spec.Command = strings.TrimSpace(opts.Command)
		spec.Args = slices.Clone(opts.Args)
		if spec.Name == "" {
			spec.Name = "custom"
		}
	} else if len(opts.Args) > 0 {
		spec.Args = slices.Clone(opts.Args)
	}
	if err := spec.Validate(); err != nil {
		return AgentSpec{}, err
	}
	return cloneSpec(spec), nil
}

func (s AgentSpec) Validate() error {
	command := strings.TrimSpace(s.Command)
	if command == "" {
		return ErrMissingCommand
	}
	if command != s.Command {
		return fmt.Errorf("%w: command must not have leading or trailing whitespace", ErrShellCommand)
	}
	if looksShellCommand(command) {
		return fmt.Errorf("%w: %q", ErrShellCommand, command)
	}
	for _, arg := range s.Args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("acp client arg contains control character")
		}
	}
	return nil
}

func (s AgentSpec) Fingerprint() string {
	args := slices.Clone(s.Args)
	if args == nil {
		args = []string{}
	}
	payload := struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{
		Command: strings.TrimSpace(s.Command),
		Args:    args,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func cloneSpec(spec AgentSpec) AgentSpec {
	spec.Args = slices.Clone(spec.Args)
	spec.EnvKeys = slices.Clone(spec.EnvKeys)
	return spec
}

func looksShellCommand(command string) bool {
	if strings.ContainsAny(command, ";&|<>`$\n\r\x00") {
		return true
	}
	if filepath.IsAbs(command) {
		return false
	}
	return strings.IndexFunc(command, unicode.IsSpace) >= 0
}
