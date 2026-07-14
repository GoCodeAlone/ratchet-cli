package acpclient

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestStoreLockPhysicalPathIsStableWhenParentIsCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "events")
	logicalPath := filepath.Join(dir, "session.ndjson.lock")
	before, err := storeLockPhysicalPath(logicalPath)
	if err != nil {
		t.Fatalf("storeLockPhysicalPath before parent creation: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll parent: %v", err)
	}
	after, err := storeLockPhysicalPath(logicalPath)
	if err != nil {
		t.Fatalf("storeLockPhysicalPath after parent creation: %v", err)
	}
	if after != before {
		t.Fatalf("physical lock path changed after parent creation:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestStoreLockPhysicalPathFoldsCaseOnConservativePlatforms(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("case folding is conservative only on Darwin and Windows")
	}
	dir := t.TempDir()
	upper, err := storeLockPhysicalPath(filepath.Join(dir, "Session.JSON.Lock"))
	if err != nil {
		t.Fatalf("storeLockPhysicalPath upper: %v", err)
	}
	lower, err := storeLockPhysicalPath(filepath.Join(dir, "session.json.lock"))
	if err != nil {
		t.Fatalf("storeLockPhysicalPath lower: %v", err)
	}
	if upper != lower {
		t.Fatalf("case variants map to different physical locks:\nupper: %s\nlower: %s", upper, lower)
	}
}

func TestStoreLockCaseFoldingPolicy(t *testing.T) {
	for _, tc := range []struct {
		goos string
		want bool
	}{
		{goos: "darwin", want: true},
		{goos: "windows", want: true},
		{goos: "linux", want: false},
		{goos: "freebsd", want: false},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			if got := storeLockFoldsCase(tc.goos); got != tc.want {
				t.Fatalf("storeLockFoldsCase(%q) = %t, want %t", tc.goos, got, tc.want)
			}
		})
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

func requireStoreLockPhysicalPath(t *testing.T, logicalPath string) string {
	t.Helper()
	physicalPath, err := storeLockPhysicalPath(logicalPath)
	if err != nil {
		t.Fatalf("storeLockPhysicalPath: %v", err)
	}
	return physicalPath
}
