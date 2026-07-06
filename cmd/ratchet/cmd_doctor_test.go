package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunDoctorJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer

	if err := runDoctor([]string{"--json"}, &stdout); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode doctor json %q: %v", stdout.String(), err)
	}
	for _, key := range []string{"version", "executable", "config_path", "data_dir", "state_dir", "daemon_status"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("doctor json missing key %q: %#v", key, payload)
		}
	}
}

func TestRunDoctorTextStaysCredentialFree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer

	if err := runDoctor(nil, &stdout); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"Ratchet doctor", "version:", "daemon:", "config:", "state:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"api_key", "token", "secret"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Fatalf("doctor output contains credential-looking text %q:\n%s", banned, out)
		}
	}
}
