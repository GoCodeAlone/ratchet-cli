package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := EnsureDataDir(); err != nil {
		t.Fatal(err)
	}

	expected := []string{"plugins", "skills", "agents"}
	for _, sub := range expected {
		p := filepath.Join(tmp, ".ratchet", sub)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected dir %s to exist", p)
		}
	}
}

func TestPIDFileRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
	if err := WritePID(); err != nil {
		t.Fatal(err)
	}
	pid, err := ReadPID()
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Errorf("got pid %d, want %d", pid, os.Getpid())
	}
}

func TestIsRunning(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	EnsureDataDir()

	// No PID file → not running
	if IsRunning() {
		t.Error("expected not running with no pid file")
	}

	// Write current PID → running
	WritePID()
	if !IsRunning() {
		t.Error("expected running with current pid")
	}

	// Write bogus PID → not running
	os.WriteFile(PIDPath(), []byte("99999999"), 0600)
	// This may or may not be running depending on OS; just ensure no panic
	_ = IsRunning()
}
