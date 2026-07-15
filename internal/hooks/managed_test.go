package hooks

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestLoadManagedPolicyMissingFile(t *testing.T) {
	policy, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: "/managed/hooks.yaml",
		ManagedReadFile: func(string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("LoadManagedPolicy: %v", err)
	}
	if policy != nil {
		t.Fatalf("policy = %#v, want nil", policy)
	}
}

func TestLoadManagedPolicyEnforcesMaximumSizeAfterReaderSeam(t *testing.T) {
	const path = "/managed/size-limit.yaml"
	atLimit := managedPolicyDocumentAtSize(t, maxManagedPolicyBytes)
	policy, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: path,
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte(atLimit), nil
		},
	})
	if err != nil {
		t.Fatalf("policy at limit: %v", err)
	}
	if policy == nil || policy.Mode != ManagedModeAdditive {
		t.Fatalf("policy at limit = %#v", policy)
	}

	oversize := atLimit + "x"
	_, err = LoadManagedPolicy(LoadOptions{
		ManagedPath: path,
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte(oversize), nil
		},
	})
	assertManagedPolicySizeError(t, err, path)
}

func TestLoadManagedPolicyOversizedDocumentsHaveStaticPrivateDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		forbidden string
	}{
		{
			name:      "first document",
			document:  managedPolicyDocumentAtSizeWithPrefix(t, maxManagedPolicyBytes+1, "SECRET-OVERSIZED-FIRST"),
			forbidden: "SECRET-OVERSIZED-FIRST",
		},
		{
			name: "trailing document",
			document: padManagedPolicyDocument(
				"mode: additive\nhooks: {}\n---\nSECRET-OVERSIZED-TRAILING: ",
				maxManagedPolicyBytes+1,
			),
			forbidden: "SECRET-OVERSIZED-TRAILING",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/oversized.yaml"
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: path,
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.document), nil
				},
			})
			assertManagedPolicySizeError(t, err, path)
			assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden)
		})
	}
}

func TestReadManagedPolicySnapshotRejectsUnstableInput(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		expected   uint64
		revalidate func() error
	}{
		{
			name:     "short read",
			content:  "ab",
			expected: 3,
			revalidate: func() error {
				t.Fatal("short read invoked post-read validation")
				return nil
			},
		},
		{
			name:     "growing read",
			content:  "abcd",
			expected: 3,
			revalidate: func() error {
				t.Fatal("growing read invoked post-read validation")
				return nil
			},
		},
		{
			name:     "metadata changed",
			content:  "abc",
			expected: 3,
			revalidate: func() error {
				return errManagedPolicyChanged
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readManagedPolicySnapshot(strings.NewReader(test.content), test.expected, test.revalidate)
			if !errors.Is(err, errManagedPolicyChanged) {
				t.Fatalf("error = %v, want errManagedPolicyChanged", err)
			}
			if err.Error() != "managed policy changed while reading" {
				t.Fatalf("error = %q, want static snapshot classification", err)
			}
		})
	}
}

func TestReadManagedPolicySnapshotInvokesPostReadValidation(t *testing.T) {
	called := 0
	data, err := readManagedPolicySnapshot(strings.NewReader("abc"), 3, func() error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("readManagedPolicySnapshot: %v", err)
	}
	if string(data) != "abc" {
		t.Fatalf("data = %q, want abc", data)
	}
	if called != 1 {
		t.Fatalf("post-read validations = %d, want 1", called)
	}
}

func TestFinishManagedPolicyReadJoinsCloseErrors(t *testing.T) {
	readErr := errors.New("read failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name      string
		initial   error
		closeErr  error
		wantRead  bool
		wantClose bool
		wantData  bool
	}{
		{name: "successful lifecycle", wantData: true},
		{name: "close failure clears data", closeErr: closeErr, wantClose: true},
		{name: "read and close failure join", initial: readErr, closeErr: closeErr, wantRead: true, wantClose: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte("policy")
			err := test.initial
			finishManagedPolicyRead(&data, &err, managedPolicyCloserFunc(func() error {
				return test.closeErr
			}))
			if (data != nil) != test.wantData {
				t.Fatalf("data = %q, wantData %v", data, test.wantData)
			}
			if errors.Is(err, readErr) != test.wantRead {
				t.Fatalf("read error membership = %v, want %v: %v", errors.Is(err, readErr), test.wantRead, err)
			}
			if errors.Is(err, closeErr) != test.wantClose {
				t.Fatalf("close error membership = %v, want %v: %v", errors.Is(err, closeErr), test.wantClose, err)
			}
			if test.closeErr != nil && err == nil {
				t.Fatal("close failure returned successful result")
			}
		})
	}
}

func TestReadManagedPolicySnapshotStopsAtMaximumPlusOne(t *testing.T) {
	reader := &trackingManagedPolicyReader{
		reader: strings.NewReader(strings.Repeat("x", maxManagedPolicyBytes+1024)),
	}
	_, err := readManagedPolicySnapshot(reader, maxManagedPolicyBytes, func() error {
		t.Fatal("growing read invoked post-read validation")
		return nil
	})
	if !errors.Is(err, errManagedPolicyChanged) {
		t.Fatalf("error = %v, want errManagedPolicyChanged", err)
	}
	if reader.bytesRead != maxManagedPolicyBytes+1 {
		t.Fatalf("bytes read = %d, want %d", reader.bytesRead, maxManagedPolicyBytes+1)
	}
}

func TestLoadManagedPolicyAnnotatesHooks(t *testing.T) {
	const path = "/managed/hooks.yaml"
	policy := loadManagedPolicyFixture(t, path, `
mode: additive
hooks:
  pre-command:
    - command: "echo managed"
`)

	if policy.Mode != ManagedModeAdditive {
		t.Fatalf("mode = %q, want %q", policy.Mode, ManagedModeAdditive)
	}
	hook := policy.Hooks.Hooks[PreCommand][0]
	if hook.SourceKind != SourceManaged {
		t.Fatalf("SourceKind = %q, want %q", hook.SourceKind, SourceManaged)
	}
	if hook.SourcePath != path {
		t.Fatalf("SourcePath = %q, want %q", hook.SourcePath, path)
	}
	if !hook.Trusted || hook.Disabled || hook.Hash == "" {
		t.Fatalf("managed hook metadata = trusted:%v disabled:%v hash:%q", hook.Trusted, hook.Disabled, hook.Hash)
	}
}

func TestLoadManagedPolicyFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "missing mode", yaml: "hooks: {}\n"},
		{name: "unknown mode", yaml: "mode: optional\nhooks: {}\n"},
		{name: "unknown event", yaml: "mode: additive\nhooks:\n  not-an-event:\n    - command: echo\n"},
		{name: "unknown field", yaml: "mode: additive\nhooks:\n  pre-command:\n    - command: echo\n      shell: sh\n"},
		{name: "malformed yaml", yaml: "mode: additive\nhooks: [\n"},
		{name: "multiple documents", yaml: "mode: additive\nhooks: {}\n---\nmode: managed-only\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: "/managed/hooks.yaml",
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.yaml), nil
				},
			})
			if !errors.Is(err, ErrManagedPolicy) {
				t.Fatalf("error = %v, want ErrManagedPolicy", err)
			}
		})
	}
}

func TestLoadManagedPolicyErrorDoesNotExposeCommand(t *testing.T) {
	const secretCommand = "echo DO-NOT-EXPOSE"
	_, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: "/managed/hooks.yaml",
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte("mode: additive\nhooks:\n  pre-command:\n    - command: " + secretCommand + "\n      invalid: true\n"), nil
		},
	})
	if !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("error = %v, want ErrManagedPolicy", err)
	}
	if strings.Contains(err.Error(), secretCommand) || strings.Contains(err.Error(), "DO-NOT-EXPOSE") {
		t.Fatalf("error exposed managed command: %v", err)
	}
}

func TestLoadManagedPolicySanitizesFieldDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		wantField string
		forbidden []string
	}{
		{
			name: "unknown hook field",
			document: `mode: additive
hooks:
  pre-command:
    - command: "echo COMMAND-SENTINEL"
      SECRET-UNKNOWN-HOOK-KEY: "SECRET-VALUE-SENTINEL"
`,
			wantField: `field "<unknown>" at line 5`,
			forbidden: []string{"SECRET-UNKNOWN-HOOK-KEY", "SECRET-VALUE-SENTINEL"},
		},
		{
			name: "unknown root field",
			document: `mode: additive
SECRET-UNKNOWN-ROOT-KEY: "SECRET-ROOT-VALUE"
hooks: {}
`,
			wantField: `field "<unknown>" at line 2`,
			forbidden: []string{"SECRET-UNKNOWN-ROOT-KEY", "SECRET-ROOT-VALUE"},
		},
		{
			name: "malformed field type",
			document: `mode: additive
hooks:
  pre-command:
    - command: "echo COMMAND-SENTINEL"
      glob: ["SECRET-TYPE-SENTINEL"]
`,
			wantField: `field "glob" at line 5`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/safe-diagnostic.yaml"
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: path,
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.document), nil
				},
			})
			assertManagedPolicyDiagnostic(t, err, path, test.wantField)
			assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden...)
		})
	}
}

func TestLoadManagedPolicySanitizesSyntaxDiagnostic(t *testing.T) {
	const path = "/managed/syntax-diagnostic.yaml"
	_, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: path,
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte("mode: additive\nhooks: [\n  SECRET-SYNTAX-SENTINEL\n"), nil
		},
	})
	assertManagedPolicyDiagnostic(t, err, path, "document line ")
}

func TestLoadManagedPolicyComplexYAMLDiagnosticsArePrivateAndBounded(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		context   string
		forbidden string
	}{
		{
			name: "deep sequence",
			document: "mode: additive\nhooks:\n  pre-command:\n    - command: " +
				strings.Repeat("[", 128) + `"SECRET-DEEP-VALUE"` + strings.Repeat("]", 128) + "\n",
			context:   `field "command" at line 4`,
			forbidden: "SECRET-DEEP-VALUE",
		},
		{
			name: "alias",
			document: `mode: &SECRET-ALIAS additive
hooks:
  pre-command:
    - command: *SECRET-ALIAS
`,
			context:   `field "command" at line 4`,
			forbidden: "SECRET-ALIAS",
		},
		{
			name: "merge",
			document: `mode: additive
hooks:
  pre-command:
    - <<: {command: "SECRET-MERGE-VALUE"}
`,
			context:   `field "<unknown>" at line 4`,
			forbidden: "SECRET-MERGE-VALUE",
		},
		{
			name: "tag",
			document: `mode: additive
hooks:
  pre-command:
    - command: !SECRET-TAG SECRET-TAG-VALUE
`,
			context:   `field "command" at line 4`,
			forbidden: "SECRET-TAG",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/complex.yaml"
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: path,
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.document), nil
				},
			})
			assertManagedPolicyDiagnostic(t, err, path, test.context)
			assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden)
		})
	}
}

func TestLoadManagedPolicyRejectsTaggedMappingKeysPrivately(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		context   string
		forbidden string
	}{
		{
			name: "root key",
			document: `!SECRET-ROOT-TAG mode: additive
hooks: {}
`,
			context:   `field "mode" at line 1: expected a string field name`,
			forbidden: "SECRET-ROOT-TAG",
		},
		{
			name: "event key",
			document: `mode: additive
hooks:
  !SECRET-EVENT-TAG pre-command:
    - command: echo safe
`,
			context:   `field "hooks.<event>" at line 3: expected a string event name`,
			forbidden: "SECRET-EVENT-TAG",
		},
		{
			name: "hook key",
			document: `mode: additive
hooks:
  pre-command:
    - !SECRET-HOOK-TAG command: SECRET-HOOK-VALUE
`,
			context:   `field "command" at line 4: expected a string hook field`,
			forbidden: "SECRET-HOOK",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/tagged-keys.yaml"
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: path,
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.document), nil
				},
			})
			assertManagedPolicyDiagnostic(t, err, path, test.context)
			assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden)
		})
	}
}

func TestLoadManagedPolicyRejectsTaggedCollectionsPrivately(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		context   string
		forbidden string
	}{
		{
			name: "root mapping",
			document: `!SECRET-ROOT-MAP
mode: additive
hooks: {}
`,
			context:   "document line 1: expected a mapping document",
			forbidden: "SECRET-ROOT-MAP",
		},
		{
			name: "hooks mapping",
			document: `mode: additive
hooks: !SECRET-HOOKS-MAP
  pre-command:
    - command: echo safe
`,
			context:   `field "hooks" at line 2: expected an event mapping`,
			forbidden: "SECRET-HOOKS-MAP",
		},
		{
			name: "event sequence",
			document: `mode: additive
hooks:
  pre-command: !SECRET-EVENT-SEQ
    - command: echo safe
`,
			context:   `field "pre-command" at line 3: expected a hook sequence`,
			forbidden: "SECRET-EVENT-SEQ",
		},
		{
			name: "hook mapping",
			document: `mode: additive
hooks:
  pre-command:
    - !SECRET-HOOK-MAP
      command: SECRET-HOOK-VALUE
`,
			context:   `field "pre-command" at line 4: expected a hook mapping`,
			forbidden: "SECRET-HOOK",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/tagged-collections.yaml"
			_, err := LoadManagedPolicy(LoadOptions{
				ManagedPath: path,
				ManagedReadFile: func(string) ([]byte, error) {
					return []byte(test.document), nil
				},
			})
			assertManagedPolicyDiagnostic(t, err, path, test.context)
			assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden)
		})
	}
}

func TestLoadManagedPolicyUnknownEventDiagnosticIsDeterministicAndPrivate(t *testing.T) {
	const (
		path        = "/managed/unknown-events.yaml"
		firstEvent  = "SECRET-FIRST-EVENT-" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		secondEvent = "SECRET-SECOND-EVENT"
		command     = "echo SECRET-EVENT-COMMAND"
	)
	document := "mode: additive\nhooks:\n  " + firstEvent + ":\n    - command: \"" + command + "\"\n  " + secondEvent + ":\n    - command: echo second\n"

	var firstError string
	for range 20 {
		_, err := LoadManagedPolicy(LoadOptions{
			ManagedPath: path,
			ManagedReadFile: func(string) ([]byte, error) {
				return []byte(document), nil
			},
		})
		assertManagedPolicyDiagnostic(t, err, path, `field "hooks.<event>" at line 3`)
		assertManagedPolicyErrorPrivateAndBounded(t, err, firstEvent, secondEvent, command)
		if firstError == "" {
			firstError = err.Error()
		} else if err.Error() != firstError {
			t.Fatalf("diagnostic changed across loads:\nfirst: %s\nnext:  %s", firstError, err)
		}
	}
}

func TestLoadManagedPolicyDuplicateAndTrailingDiagnosticsArePrivate(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		context   string
		forbidden []string
	}{
		{
			name:      "duplicate field",
			document:  "mode: additive\nmode: SECRET-DUPLICATE-MODE\nhooks: {}\n",
			context:   `field "mode" at line 2: duplicate field`,
			forbidden: []string{"SECRET-DUPLICATE-MODE"},
		},
		{
			name: "duplicate event",
			document: `mode: additive
hooks:
  pre-command:
    - command: echo first
  pre-command:
    - command: echo SECRET-DUPLICATE-EVENT
`,
			context:   `field "pre-command" at line 5: duplicate event`,
			forbidden: []string{"SECRET-DUPLICATE-EVENT"},
		},
		{
			name: "trailing document",
			document: `mode: additive
hooks: {}
---
SECRET-TRAILING-KEY: SECRET-TRAILING-VALUE
`,
			context:   "document line 3: multiple YAML documents",
			forbidden: []string{"SECRET-TRAILING-KEY", "SECRET-TRAILING-VALUE"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const path = "/managed/duplicate-diagnostics.yaml"
			var firstError string
			for range 10 {
				_, err := LoadManagedPolicy(LoadOptions{
					ManagedPath: path,
					ManagedReadFile: func(string) ([]byte, error) {
						return []byte(test.document), nil
					},
				})
				assertManagedPolicyDiagnostic(t, err, path, test.context)
				assertManagedPolicyErrorPrivateAndBounded(t, err, test.forbidden...)
				if firstError == "" {
					firstError = err.Error()
				} else if err.Error() != firstError {
					t.Fatalf("diagnostic changed across loads:\nfirst: %s\nnext:  %s", firstError, err)
				}
			}
		})
	}
}

func TestLoadManagedPolicyRejectsDuplicateHookHash(t *testing.T) {
	_, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: "/managed/hooks.yaml",
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte(`
mode: additive
hooks:
  pre-command:
    - command: "echo duplicate"
    - command: "echo duplicate"
`), nil
		},
	})
	if !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("error = %v, want ErrManagedPolicy", err)
	}
}

func TestManagedWindowsACEPolicyRejectsUnhandledGrantForms(t *testing.T) {
	const (
		accessAllowedACEType       = 0x0
		accessDeniedACEType        = 0x1
		accessAllowedObjectACEType = 0x5
	)

	if err := validateManagedWindowsACE(accessAllowedObjectACEType, false, true, false); err == nil {
		t.Fatal("write-capable object allow ACE was accepted")
	}
	if err := validateManagedWindowsACE(accessAllowedACEType, false, false, false); err != nil {
		t.Fatalf("ordinary read-only Users ACE: %v", err)
	}
	if err := validateManagedWindowsACE(accessDeniedACEType, false, true, false); err != nil {
		t.Fatalf("basic deny ACE: %v", err)
	}
}

func TestManagedWindowsACEPolicyRejectsCallbackAllowWrite(t *testing.T) {
	const accessAllowedCallbackACEType = 0x9
	if err := validateManagedWindowsACE(accessAllowedCallbackACEType, false, true, false); err == nil {
		t.Fatal("write-capable callback allow ACE was accepted")
	}
}

func TestManagedWindowsDACLPolicyRequiresProtection(t *testing.T) {
	if err := validateManagedWindowsDACLProtection(true); err != nil {
		t.Fatalf("protected DACL: %v", err)
	}
	if err := validateManagedWindowsDACLProtection(false); err == nil {
		t.Fatal("unprotected DACL was accepted")
	}
}

func TestManagedWindowsDescriptorSerializationRejectsEmpty(t *testing.T) {
	serialized, err := validateManagedWindowsDescriptorSerialization("protected-admin")
	if err != nil || serialized != "protected-admin" {
		t.Fatalf("valid serialization = %q, %v", serialized, err)
	}
	serialized, err = validateManagedWindowsDescriptorSerialization("")
	if err == nil {
		t.Fatal("empty security descriptor serialization was accepted")
	}
	if serialized != "" || err.Error() != "managed policy security descriptor serialization failed" {
		t.Fatalf("empty serialization = %q, %v", serialized, err)
	}
}

func TestApplyManagedPolicyNormalizesDirectAdditiveHooksWithoutMutation(t *testing.T) {
	input := Hook{
		Command:             "echo managed",
		CommandWindows:      "echo managed",
		Glob:                "*.go",
		Event:               PostCommand,
		SourceKind:          SourceUser,
		SourceID:            "caller:source",
		SourcePath:          "/caller/path",
		PluginName:          "caller-plugin",
		PluginVersion:       "caller-version",
		Hash:                "caller-hash",
		Disabled:            true,
		Suppressed:          true,
		UnsupportedPlatform: true,
	}
	policy := &ManagedPolicy{
		Mode: ManagedModeAdditive,
		Hooks: HookConfig{Hooks: map[Event][]Hook{
			PreCommand: {input},
		}},
	}
	cfg := hookConfigWithSources(SourceUser)

	cfg.ApplyManagedPolicy(policy)

	if got := policy.Hooks.Hooks[PreCommand][0]; got != input {
		t.Fatalf("caller-owned hook mutated:\n got: %#v\nwant: %#v", got, input)
	}
	hooks := cfg.Hooks[PreCommand]
	if len(hooks) != 2 {
		t.Fatalf("hooks = %d, want local and managed hooks", len(hooks))
	}
	if hooks[0].Suppressed || !hooks[0].runnable() {
		t.Fatalf("local additive hook = %#v, want runnable", hooks[0])
	}
	managed := hooks[1]
	if managed.Event != PreCommand || managed.SourceKind != SourceManaged || managed.SourceID != "managed:managed-hooks.yaml" {
		t.Fatalf("managed source metadata = %#v", managed)
	}
	if managed.SourcePath != "" || managed.PluginName != "" || managed.PluginVersion != "" {
		t.Fatalf("managed caller metadata was retained: %#v", managed)
	}
	if !managed.Trusted || managed.Disabled || managed.Suppressed || managed.UnsupportedPlatform || !managed.runnable() {
		t.Fatalf("managed execution metadata = %#v, want runnable and trusted", managed)
	}
	if managed.Hash == "" || managed.Hash != managed.DescriptorHash() {
		t.Fatalf("managed hash = %q, want normalized descriptor hash %q", managed.Hash, managed.DescriptorHash())
	}
}

func TestApplyManagedPolicyDirectManagedOnlyRunsManagedHooks(t *testing.T) {
	cfg := hookConfigWithSources(SourceProject)
	policy := &ManagedPolicy{
		Mode: ManagedModeOnly,
		Hooks: HookConfig{Hooks: map[Event][]Hook{
			PreCommand: {{Command: "echo managed", CommandWindows: "echo managed"}},
		}},
	}

	cfg.ApplyManagedPolicy(policy)

	hooks := cfg.Hooks[PreCommand]
	if len(hooks) != 2 {
		t.Fatalf("hooks = %d, want local and managed hooks", len(hooks))
	}
	if !hooks[0].Suppressed || hooks[0].runnable() {
		t.Fatalf("local hook = %#v, want suppressed", hooks[0])
	}
	if hooks[1].SourceKind != SourceManaged || hooks[1].Suppressed || !hooks[1].runnable() {
		t.Fatalf("managed hook = %#v, want runnable", hooks[1])
	}
}

func TestApplyManagedPolicyDirectRepeatedApplicationReplacesManagedHooks(t *testing.T) {
	cfg := hookConfigWithSources(SourceUser)
	policy := &ManagedPolicy{
		Mode: ManagedModeAdditive,
		Hooks: HookConfig{Hooks: map[Event][]Hook{
			PreCommand: {{Command: "echo managed", CommandWindows: "echo managed"}},
		}},
	}

	cfg.ApplyManagedPolicy(policy)
	cfg.ApplyManagedPolicy(policy)

	hooks := cfg.Hooks[PreCommand]
	if len(hooks) != 2 {
		t.Fatalf("hooks after repeated application = %d, want local and one managed hook", len(hooks))
	}
	if got := sourceKinds(hooks); !slices.Equal(got, []SourceKind{SourceUser, SourceManaged}) {
		t.Fatalf("source order after repeated application = %v", got)
	}
}

func TestApplyManagedPolicyAdditivePreservesArbitrarySources(t *testing.T) {
	cfg := hookConfigWithSources(SourceUser, SourceProject, SourcePlugin)
	policy := loadManagedPolicyFixture(t, "/managed/hooks.yaml", `
mode: additive
hooks:
  pre-command:
    - command: "echo managed"
`)

	cfg.ApplyManagedPolicy(policy)

	hooks := cfg.Hooks[PreCommand]
	if len(hooks) != 4 {
		t.Fatalf("hooks = %d, want 4", len(hooks))
	}
	for _, hook := range hooks {
		if hook.Suppressed {
			t.Fatalf("%s hook is suppressed in additive mode", hook.SourceKind)
		}
	}
	if got := sourceKinds(hooks); !slices.Equal(got, []SourceKind{SourceUser, SourceProject, SourcePlugin, SourceManaged}) {
		t.Fatalf("source order = %v", got)
	}
}

func TestApplyManagedPolicyManagedOnlyPreservesAndSuppressesArbitrarySources(t *testing.T) {
	cfg := hookConfigWithSources(SourcePlugin, SourceUser, SourceProject)
	policy := loadManagedPolicyFixture(t, "/managed/hooks.yaml", `
mode: managed-only
hooks:
  pre-command:
    - command: "echo managed"
`)

	cfg.ApplyManagedPolicy(policy)

	hooks := cfg.Hooks[PreCommand]
	if len(hooks) != 4 {
		t.Fatalf("hooks = %d, want all sources preserved", len(hooks))
	}
	for _, hook := range hooks {
		wantSuppressed := hook.SourceKind != SourceManaged
		if hook.Suppressed != wantSuppressed {
			t.Fatalf("%s suppressed = %v, want %v", hook.SourceKind, hook.Suppressed, wantSuppressed)
		}
		if hook.runnable() == wantSuppressed {
			t.Fatalf("%s runnable = %v, suppressed = %v", hook.SourceKind, hook.runnable(), hook.Suppressed)
		}
	}
}

func TestApplyManagedPolicyInvalidModesFailClosed(t *testing.T) {
	tests := []struct {
		name string
		mode ManagedMode
	}{
		{name: "zero mode"},
		{name: "unknown mode", mode: "SECRET-UNKNOWN-MODE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := hookConfigWithSources(SourceUser, SourceProject, SourcePlugin)
			cfg.ApplyManagedPolicy(&ManagedPolicy{
				Mode:  test.mode,
				Hooks: HookConfig{Hooks: make(map[Event][]Hook)},
			})
			for _, hook := range cfg.Hooks[PreCommand] {
				if !hook.Suppressed || hook.runnable() {
					t.Fatalf("%s hook remained runnable for invalid mode %q", hook.SourceKind, test.mode)
				}
			}
		})
	}
}

func TestApplyManagedPolicyCanSwitchBackToAdditive(t *testing.T) {
	cfg := hookConfigWithSources(SourceUser)
	managedOnly := loadManagedPolicyFixture(t, "/managed/hooks.yaml", "mode: managed-only\nhooks: {}\n")
	additive := loadManagedPolicyFixture(t, "/managed/hooks.yaml", "mode: additive\nhooks: {}\n")

	cfg.ApplyManagedPolicy(managedOnly)
	if !cfg.Hooks[PreCommand][0].Suppressed {
		t.Fatal("user hook was not suppressed")
	}
	cfg.ApplyManagedPolicy(additive)
	if cfg.Hooks[PreCommand][0].Suppressed {
		t.Fatal("user hook remained suppressed after additive policy")
	}
}

func loadManagedPolicyFixture(t *testing.T, path, document string) *ManagedPolicy {
	t.Helper()
	policy, err := LoadManagedPolicy(LoadOptions{
		ManagedPath: path,
		ManagedReadFile: func(string) ([]byte, error) {
			return []byte(document), nil
		},
	})
	if err != nil {
		t.Fatalf("LoadManagedPolicy: %v", err)
	}
	return policy
}

func hookConfigWithSources(kinds ...SourceKind) *HookConfig {
	hooks := make([]Hook, 0, len(kinds))
	for _, kind := range kinds {
		hook := Hook{
			Command:    "echo " + string(kind),
			Event:      PreCommand,
			SourceKind: kind,
			SourceID:   string(kind) + ":fixture",
			Trusted:    true,
		}
		hook.Hash = hook.DescriptorHash()
		hooks = append(hooks, hook)
	}
	return &HookConfig{Hooks: map[Event][]Hook{PreCommand: hooks}}
}

func sourceKinds(hooks []Hook) []SourceKind {
	return slices.Collect(func(yield func(SourceKind) bool) {
		for _, hook := range hooks {
			if !yield(hook.SourceKind) {
				return
			}
		}
	})
}

func assertManagedPolicyDiagnostic(t *testing.T, err error, path, context string) {
	t.Helper()
	if !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("error = %v, want ErrManagedPolicy", err)
	}
	for _, fragment := range []string{path, context} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want safe context %q", err, fragment)
		}
	}
	for _, sentinel := range []string{
		"COMMAND-SENTINEL",
		"SECRET-VALUE-SENTINEL",
		"SECRET-TYPE-SENTINEL",
		"SECRET-SYNTAX-SENTINEL",
	} {
		if strings.Contains(err.Error(), sentinel) {
			t.Fatalf("error exposed %q: %v", sentinel, err)
		}
	}
}

func assertManagedPolicyErrorPrivateAndBounded(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	if len(err.Error()) > 256 {
		t.Fatalf("diagnostic length = %d, want <= 256: %q", len(err.Error()), err)
	}
	for _, value := range forbidden {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("error exposed attacker-controlled scalar %q: %v", value, err)
		}
	}
}

func assertManagedPolicySizeError(t *testing.T, err error, path string) {
	t.Helper()
	if !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("error = %v, want ErrManagedPolicy", err)
	}
	want := ErrManagedPolicy.Error() + ": read " + path + ": managed policy exceeds maximum size"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func managedPolicyDocumentAtSize(t *testing.T, size int) string {
	t.Helper()
	return managedPolicyDocumentAtSizeWithPrefix(t, size, "")
}

func managedPolicyDocumentAtSizeWithPrefix(t *testing.T, size int, commandPrefix string) string {
	t.Helper()
	const (
		prefix = "mode: additive\nhooks:\n  pre-command:\n    - command: \""
		suffix = "\"\n"
	)
	if size < len(prefix)+len(commandPrefix)+len(suffix) {
		t.Fatalf("policy size %d is too small", size)
	}
	return prefix + commandPrefix + strings.Repeat("x", size-len(prefix)-len(commandPrefix)-len(suffix)) + suffix
}

func padManagedPolicyDocument(prefix string, size int) string {
	if len(prefix) >= size {
		return prefix[:size]
	}
	return prefix + strings.Repeat("x", size-len(prefix))
}

type trackingManagedPolicyReader struct {
	reader    *strings.Reader
	bytesRead int
}

type managedPolicyCloserFunc func() error

func (f managedPolicyCloserFunc) Close() error {
	return f()
}

func (r *trackingManagedPolicyReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}
