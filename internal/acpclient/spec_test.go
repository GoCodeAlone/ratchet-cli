package acpclient

import (
	"errors"
	"testing"
)

func TestDefaultRegistryHasCommonAgentTemplates(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"ratchet", "codex", "claude", "gemini", "opencode", "custom"} {
		t.Run(name, func(t *testing.T) {
			spec, ok := reg.Lookup(name)
			if !ok {
				t.Fatalf("Lookup(%q) ok = false, want true", name)
			}
			if spec.Name != name {
				t.Fatalf("Lookup(%q).Name = %q, want %q", name, spec.Name, name)
			}
		})
	}
}

func TestRegistryResolvePreservesExplicitCommandAndArgs(t *testing.T) {
	spec, err := DefaultRegistry().Resolve(RunOptions{
		Agent:   "codex",
		Command: "/tmp/acp-agent",
		Args:    []string{"--stdio", "--profile=work"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if spec.Command != "/tmp/acp-agent" {
		t.Fatalf("Command = %q, want explicit override", spec.Command)
	}
	wantArgs := []string{"--stdio", "--profile=work"}
	if len(spec.Args) != len(wantArgs) {
		t.Fatalf("Args = %#v, want %#v", spec.Args, wantArgs)
	}
	for i := range wantArgs {
		if spec.Args[i] != wantArgs[i] {
			t.Fatalf("Args[%d] = %q, want %q", i, spec.Args[i], wantArgs[i])
		}
	}
}

func TestRegistryResolveRejectsShellCommandStrings(t *testing.T) {
	_, err := DefaultRegistry().Resolve(RunOptions{Command: "codex acp"})
	if !errors.Is(err, ErrShellCommand) {
		t.Fatalf("Resolve error = %v, want ErrShellCommand", err)
	}

	_, err = DefaultRegistry().Resolve(RunOptions{Command: "codex; rm -rf ."})
	if !errors.Is(err, ErrShellCommand) {
		t.Fatalf("Resolve metachar error = %v, want ErrShellCommand", err)
	}
}

func TestRegistryResolveReportsMissingCommand(t *testing.T) {
	_, err := DefaultRegistry().Resolve(RunOptions{Agent: "custom"})
	if !errors.Is(err, ErrMissingCommand) {
		t.Fatalf("Resolve error = %v, want ErrMissingCommand", err)
	}
}

func TestRegistryResolveTrimsExplicitCommand(t *testing.T) {
	spec, err := DefaultRegistry().Resolve(RunOptions{Command: " codex "})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if spec.Command != "codex" {
		t.Fatalf("Command = %q, want trimmed command", spec.Command)
	}
}

func TestCommandFingerprintIsStableAndArgSensitive(t *testing.T) {
	a := AgentSpec{Command: "codex", Args: []string{"acp", "--model", "gpt-5"}}
	b := AgentSpec{Command: "codex", Args: []string{"acp", "--model", "gpt-5"}}
	c := AgentSpec{Command: "codex", Args: []string{"acp", "--model", "gpt-5-mini"}}

	if a.Fingerprint() == "" {
		t.Fatal("Fingerprint returned empty string")
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("same command fingerprint mismatch: %q != %q", a.Fingerprint(), b.Fingerprint())
	}
	if a.Fingerprint() == c.Fingerprint() {
		t.Fatalf("fingerprints should differ when args differ: %q", a.Fingerprint())
	}
}

func TestCommandFingerprintNormalizesCommandAndEmptyArgs(t *testing.T) {
	a := AgentSpec{Command: " codex "}
	b := AgentSpec{Command: "codex", Args: []string{}}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("fingerprints differ for logically identical specs: %q != %q", a.Fingerprint(), b.Fingerprint())
	}
}
