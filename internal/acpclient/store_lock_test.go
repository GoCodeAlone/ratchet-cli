package acpclient

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	storeLockHelperEnv      = "RATCHET_STORE_LOCK_HELPER"
	storeLockHelperPathEnv  = "RATCHET_STORE_LOCK_PATH"
	storeLockHelperReadyEnv = "RATCHET_STORE_LOCK_READY"
)

func runSessionStoreCrossProcessLockBlocks(t *testing.T, hold func(string) (func() error, error)) {
	t.Helper()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	store := NewStore(storePath)
	if err := store.Upsert(SessionRecord{ID: "session-1"}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}
	release, err := hold(storePath + ".lock")
	if err != nil {
		t.Fatalf("hold store lock: %v", err)
	}
	defer func() { _ = release() }()

	readyPath := filepath.Join(dir, "child-ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSessionStoreLockSubprocessHelper$")
	cmd.Env = append(os.Environ(),
		storeLockHelperEnv+"=1",
		storeLockHelperPathEnv+"="+storePath,
		storeLockHelperReadyEnv+"="+readyPath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waitForStoreLockHelperReady(t, readyPath)
	select {
	case err := <-done:
		t.Fatalf("helper exited while parent held sessions lock: %v\n%s", err, output.String())
	case <-time.After(200 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatalf("release store lock: %v", err)
	}
	release = func() error { return nil }
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper after release: %v\n%s", err, output.String())
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper remained blocked after sessions lock release")
	}
}

func TestSessionStoreLockSubprocessHelper(t *testing.T) {
	if os.Getenv(storeLockHelperEnv) != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv(storeLockHelperReadyEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	if _, err := NewStore(os.Getenv(storeLockHelperPathEnv)).List(); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func waitForStoreLockHelperReady(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("store lock helper did not become ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
