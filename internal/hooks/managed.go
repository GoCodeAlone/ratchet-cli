package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrManagedPolicy identifies a present policy that could not be securely
	// loaded or validated.
	ErrManagedPolicy = errors.New("managed hook policy")
	// ErrManagedPolicyUnsupportedPlatform identifies a platform without a
	// managed-policy trust-boundary implementation.
	ErrManagedPolicyUnsupportedPlatform = errors.New("managed hook policy is unsupported on this platform")
)

// ManagedMode controls whether local hook sources may execute alongside
// administrator-managed hooks.
type ManagedMode string

const (
	ManagedModeAdditive ManagedMode = "additive"
	ManagedModeOnly     ManagedMode = "managed-only"

	// maxManagedPolicyBytes bounds secure file reads and yaml.v3 parser work.
	// One MiB is ample for hook policy; the limit is intentionally not configurable.
	maxManagedPolicyBytes = 1 << 20
	managedPolicySourceID = "managed:managed-hooks.yaml"

	managedWindowsAccessAllowedACEType uint8 = 0x0
	managedWindowsAccessDeniedACEType  uint8 = 0x1
)

var (
	errManagedPolicyTooLarge   = errors.New("managed policy exceeds maximum size")
	errManagedPolicyChanged    = errors.New("managed policy changed while reading")
	errManagedPolicyReadFailed = errors.New("managed policy read failed")
)

type managedPolicySnapshotReader func(io.Reader, uint64, func() error) ([]byte, error)

func finishManagedPolicyRead(data *[]byte, readErr *error, closer io.Closer) {
	if closeErr := closer.Close(); closeErr != nil {
		*data = nil
		*readErr = errors.Join(*readErr, closeErr)
	}
}

// ManagedPolicy is the administrator-owned hook policy document.
type ManagedPolicy struct {
	Mode  ManagedMode `yaml:"mode"`
	Hooks HookConfig  `yaml:",inline"`
}

// LoadManagedPolicy securely reads and validates the administrator-owned
// policy. A missing file is the normal unmanaged configuration.
func LoadManagedPolicy(opts LoadOptions) (*ManagedPolicy, error) {
	path := opts.ManagedPath
	if path == "" {
		var err error
		path, err = defaultManagedPolicyPath()
		if err != nil {
			return nil, fmt.Errorf("%w: resolve default path: %w", ErrManagedPolicy, err)
		}
	}

	readFile := secureReadManagedFile
	if opts.ManagedReadFile != nil {
		readFile = opts.ManagedReadFile
	}
	data, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrManagedPolicy, path, err)
	}
	if err := validateManagedPolicySize(uint64(len(data))); err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrManagedPolicy, path, err)
	}

	policy, err := decodeManagedPolicy(data)
	if err != nil {
		var diagnostic *managedPolicyDiagnostic
		if !errors.As(err, &diagnostic) {
			diagnostic = newManagedDocumentDiagnostic(1, "invalid policy document")
		}
		return nil, fmt.Errorf("%w: parse %s: %s", ErrManagedPolicy, path, diagnostic)
	}
	if policy.Mode != ManagedModeAdditive && policy.Mode != ManagedModeOnly {
		return nil, fmt.Errorf("%w: mode in %s must be additive or managed-only", ErrManagedPolicy, path)
	}
	policy.Hooks.AnnotateSource(SourceMetadata{
		Kind:           SourceManaged,
		ID:             managedPolicySourceID,
		Path:           path,
		TrustByDefault: true,
	})
	seen := make(map[string]struct{})
	for _, hooks := range policy.Hooks.Hooks {
		for _, hook := range hooks {
			if _, ok := seen[hook.Hash]; ok {
				return nil, fmt.Errorf("%w: duplicate hook in %s", ErrManagedPolicy, path)
			}
			seen[hook.Hash] = struct{}{}
		}
	}
	return policy, nil
}

func readManagedPolicySnapshot(reader io.Reader, expectedSize uint64, revalidate func() error) ([]byte, error) {
	if err := validateManagedPolicySize(expectedSize); err != nil {
		return nil, err
	}
	data := make([]byte, int(expectedSize))
	n, err := io.ReadFull(reader, data)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || n != len(data) {
		return nil, errManagedPolicyChanged
	}
	if err != nil {
		return nil, errManagedPolicyReadFailed
	}

	var probe [1]byte
	n, err = reader.Read(probe[:])
	if n != 0 || err == nil {
		return nil, errManagedPolicyChanged
	}
	if !errors.Is(err, io.EOF) {
		return nil, errManagedPolicyReadFailed
	}
	if revalidate != nil {
		if err := revalidate(); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func validateManagedPolicySize(size uint64) error {
	if size > maxManagedPolicyBytes {
		return errManagedPolicyTooLarge
	}
	return nil
}

func decodeManagedPolicy(data []byte) (*ManagedPolicy, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, newManagedDocumentDiagnostic(yamlErrorLine(err), "invalid YAML syntax")
	}
	if err := validateManagedPolicyDocument(&document); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == nil {
		return nil, newManagedDocumentDiagnostic(yamlNodeLine(&trailing), "multiple YAML documents")
	} else if !errors.Is(err, io.EOF) {
		return nil, newManagedDocumentDiagnostic(yamlErrorLine(err), "invalid YAML syntax")
	}

	var policy ManagedPolicy
	if err := document.Decode(&policy); err != nil {
		return nil, newManagedDocumentDiagnostic(yamlNodeLine(&document), "invalid policy schema")
	}
	if policy.Hooks.Hooks == nil {
		policy.Hooks.Hooks = make(map[Event][]Hook)
	}
	return &policy, nil
}

type managedPolicyDiagnostic struct {
	field   string
	line    int
	problem string
}

func (e *managedPolicyDiagnostic) Error() string {
	if e.field != "" {
		return fmt.Sprintf("field %q at line %d: %s", e.field, e.line, e.problem)
	}
	return fmt.Sprintf("document line %d: %s", e.line, e.problem)
}

func newManagedFieldDiagnostic(field string, line int, problem string) *managedPolicyDiagnostic {
	return &managedPolicyDiagnostic{field: safeManagedField(field), line: max(1, line), problem: problem}
}

func newManagedDocumentDiagnostic(line int, problem string) *managedPolicyDiagnostic {
	return &managedPolicyDiagnostic{line: max(1, line), problem: problem}
}

func validateManagedPolicyDocument(document *yaml.Node) error {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return newManagedDocumentDiagnostic(yamlNodeLine(document), "expected one mapping document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode || root.Tag != "!!map" {
		return newManagedDocumentDiagnostic(yamlNodeLine(root), "expected a mapping document")
	}
	seen := make(map[string]struct{})
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return newManagedDocumentDiagnostic(yamlNodeLine(key), "expected a string field name")
		}
		if key.Tag != "!!str" {
			return newManagedFieldDiagnostic(key.Value, key.Line, "expected a string field name")
		}
		if _, ok := seen[key.Value]; ok {
			return newManagedFieldDiagnostic(key.Value, key.Line, "duplicate field")
		}
		seen[key.Value] = struct{}{}
		switch key.Value {
		case "mode":
			if err := requireManagedStringField(key.Value, key.Line, value); err != nil {
				return err
			}
		case "hooks":
			if err := validateManagedHooksNode(key, value); err != nil {
				return err
			}
		default:
			return newManagedFieldDiagnostic(key.Value, key.Line, "unknown field")
		}
	}
	return nil
}

func validateManagedHooksNode(key, hooksNode *yaml.Node) error {
	if hooksNode.Kind != yaml.MappingNode || hooksNode.Tag != "!!map" {
		return newManagedFieldDiagnostic(key.Value, key.Line, "expected an event mapping")
	}
	seenEvents := make(map[string]struct{})
	for i := 0; i+1 < len(hooksNode.Content); i += 2 {
		event, hookList := hooksNode.Content[i], hooksNode.Content[i+1]
		if event.Kind != yaml.ScalarNode {
			return newManagedFieldDiagnostic("hooks", yamlNodeLine(event), "expected a string event name")
		}
		if event.Tag != "!!str" {
			return newManagedFieldDiagnostic("hooks.<event>", event.Line, "expected a string event name")
		}
		if !slices.Contains(AllEvents, Event(event.Value)) {
			return newManagedFieldDiagnostic("hooks.<event>", event.Line, "unsupported event")
		}
		if _, ok := seenEvents[event.Value]; ok {
			return newManagedFieldDiagnostic(event.Value, event.Line, "duplicate event")
		}
		seenEvents[event.Value] = struct{}{}
		if hookList.Kind != yaml.SequenceNode || hookList.Tag != "!!seq" {
			return newManagedFieldDiagnostic(event.Value, event.Line, "expected a hook sequence")
		}
		for _, hook := range hookList.Content {
			if err := validateManagedHookNode(event, hook); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateManagedHookNode(event, hook *yaml.Node) error {
	if hook.Kind != yaml.MappingNode || hook.Tag != "!!map" {
		return newManagedFieldDiagnostic(event.Value, yamlNodeLine(hook), "expected a hook mapping")
	}
	seen := make(map[string]struct{})
	for i := 0; i+1 < len(hook.Content); i += 2 {
		key, value := hook.Content[i], hook.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return newManagedFieldDiagnostic(event.Value, yamlNodeLine(key), "expected a string hook field")
		}
		if key.Tag != "!!str" {
			return newManagedFieldDiagnostic(key.Value, key.Line, "expected a string hook field")
		}
		if _, ok := seen[key.Value]; ok {
			return newManagedFieldDiagnostic(key.Value, key.Line, "duplicate field")
		}
		seen[key.Value] = struct{}{}
		switch key.Value {
		case "command", "command_windows", "glob":
			if err := requireManagedStringField(key.Value, key.Line, value); err != nil {
				return err
			}
		default:
			return newManagedFieldDiagnostic(key.Value, key.Line, "unknown field")
		}
	}
	return nil
}

func requireManagedStringField(field string, line int, value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
		return newManagedFieldDiagnostic(field, line, "expected a string value")
	}
	return nil
}

func safeManagedField(field string) string {
	switch field {
	case "mode", "hooks", "hooks.<event>", "command", "command_windows", "glob":
		return field
	}
	if slices.Contains(AllEvents, Event(field)) {
		return field
	}
	return "<unknown>"
}

func yamlErrorLine(err error) int {
	message := err.Error()
	_, after, ok := strings.Cut(message, "line ")
	if !ok {
		return 1
	}
	digits := after
	if end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
		digits = digits[:end]
	}
	line, parseErr := strconv.Atoi(digits)
	if parseErr != nil {
		return 1
	}
	return max(1, line)
}

func yamlNodeLine(node *yaml.Node) int {
	if node == nil {
		return 1
	}
	if node.Line > 0 {
		return node.Line
	}
	if len(node.Content) > 0 {
		return yamlNodeLine(node.Content[0])
	}
	return 1
}

func validateManagedWindowsACE(aceType uint8, inheritOnly, grantsWrite, administrative bool) error {
	if inheritOnly {
		return nil
	}
	switch aceType {
	case managedWindowsAccessDeniedACEType:
		return nil
	case managedWindowsAccessAllowedACEType:
		if !grantsWrite || administrative {
			return nil
		}
		return errors.New("managed policy grants modification to a non-administrator principal")
	default:
		return errors.New("managed policy contains an unsupported ACE type")
	}
}

func validateManagedWindowsDACLProtection(protected bool) error {
	if !protected {
		return errors.New("managed policy DACL is not protected")
	}
	return nil
}

func validateManagedWindowsDescriptorSerialization(serialized string) (string, error) {
	if serialized == "" {
		return "", errors.New("managed policy security descriptor serialization failed")
	}
	return serialized, nil
}

// ApplyManagedPolicy installs the current managed hooks after all local sources
// are assembled, preserving local hooks for diagnostics and applying mode last.
func (hc *HookConfig) ApplyManagedPolicy(policy *ManagedPolicy) {
	if hc == nil {
		return
	}
	if hc.Hooks == nil {
		hc.Hooks = make(map[Event][]Hook)
	}
	for event, hooks := range hc.Hooks {
		retained := hooks[:0]
		for _, hook := range hooks {
			if hook.SourceKind == SourceManaged {
				continue
			}
			hook.Suppressed = false
			retained = append(retained, hook)
		}
		hc.Hooks[event] = retained
	}
	if policy == nil {
		return
	}
	managedHooks := normalizeManagedPolicyHooks(policy)
	for event, hooks := range managedHooks.Hooks {
		hc.Hooks[event] = append(hc.Hooks[event], hooks...)
	}
	switch policy.Mode {
	case ManagedModeAdditive:
		return
	case ManagedModeOnly:
		// Suppression is applied below.
	default:
		// Directly constructed invalid policies fail closed as managed-only.
	}
	for event, hooks := range hc.Hooks {
		for i := range hooks {
			hooks[i].Suppressed = hooks[i].SourceKind != SourceManaged
		}
		hc.Hooks[event] = hooks
	}
}

func normalizeManagedPolicyHooks(policy *ManagedPolicy) HookConfig {
	normalized := HookConfig{Hooks: make(map[Event][]Hook, len(policy.Hooks.Hooks))}
	for event, hooks := range policy.Hooks.Hooks {
		normalizedHooks := make([]Hook, len(hooks))
		for i, hook := range hooks {
			normalizedHooks[i] = Hook{
				Command:        hook.Command,
				CommandWindows: hook.CommandWindows,
				Glob:           hook.Glob,
			}
		}
		normalized.Hooks[event] = normalizedHooks
	}
	normalized.AnnotateSource(SourceMetadata{
		Kind:           SourceManaged,
		ID:             managedPolicySourceID,
		Path:           loadedManagedPolicySourcePath(policy),
		TrustByDefault: true,
	})
	return normalized
}

func loadedManagedPolicySourcePath(policy *ManagedPolicy) string {
	var sourcePath string
	found := false
	for _, hooks := range policy.Hooks.Hooks {
		for _, hook := range hooks {
			if hook.SourceKind != SourceManaged || hook.SourceID != managedPolicySourceID {
				return ""
			}
			if !found {
				sourcePath = hook.SourcePath
				found = true
				continue
			}
			if hook.SourcePath != sourcePath {
				return ""
			}
		}
	}
	return sourcePath
}
