package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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

func TestRestartWaitsForCapturedDaemonBeforeStarting(t *testing.T) {
	const oldPID = 41
	var (
		alive       atomic.Bool
		startCalled atomic.Bool
	)
	alive.Store(true)
	shutdownCalled := make(chan struct{})
	ops := daemonLifecycleOps{
		readPID: func() (int, error) { return oldPID, nil },
		processRunning: func(pid int) bool {
			return pid == oldPID && alive.Load()
		},
		shutdownRPC: func(context.Context) error {
			close(shutdownCalled)
			return nil
		},
		terminate: func(int) error {
			return errors.New("terminate called before graceful shutdown window elapsed")
		},
		cleanupPID:    func() {},
		cleanupSocket: func() {},
		startBackground: func(bool) error {
			if alive.Load() {
				return errors.New("replacement started while captured daemon was alive")
			}
			startCalled.Store(true)
			return nil
		},
		pollInterval:   time.Millisecond,
		gracePeriod:    time.Second,
		fallbackPeriod: time.Second,
	}

	done := make(chan error, 1)
	go func() {
		done <- restartWithOps(t.Context(), false, ops)
	}()
	<-shutdownCalled
	select {
	case err := <-done:
		t.Fatalf("restart returned before old daemon exited: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	alive.Store(false)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !startCalled.Load() {
		t.Fatal("replacement daemon was not started")
	}
}

func TestStopContextEscalatesWhenGracefulShutdownDoesNotExit(t *testing.T) {
	const oldPID = 52
	var (
		alive          atomic.Bool
		shutdownCalled atomic.Bool
		terminateCalls atomic.Int32
	)
	alive.Store(true)
	ops := daemonLifecycleOps{
		readPID: func() (int, error) { return oldPID, nil },
		processRunning: func(pid int) bool {
			return pid == oldPID && alive.Load()
		},
		shutdownRPC: func(context.Context) error {
			shutdownCalled.Store(true)
			return nil
		},
		terminate: func(pid int) error {
			if pid != oldPID {
				t.Fatalf("terminate pid = %d, want %d", pid, oldPID)
			}
			if !shutdownCalled.Load() {
				t.Fatal("terminate called before graceful shutdown request")
			}
			terminateCalls.Add(1)
			alive.Store(false)
			return nil
		},
		cleanupPID:      func() {},
		cleanupSocket:   func() {},
		startBackground: func(bool) error { return nil },
		pollInterval:    time.Millisecond,
		gracePeriod:     5 * time.Millisecond,
		fallbackPeriod:  time.Second,
	}

	replaced, err := stopContextWithOps(t.Context(), ops)
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("stop reported a concurrent replacement")
	}
	if terminateCalls.Load() != 1 {
		t.Fatalf("terminate calls = %d, want 1", terminateCalls.Load())
	}
}

func TestStopContextCleansUnchangedStaleOwnership(t *testing.T) {
	const oldPID = 57
	var (
		pidCleaned    atomic.Bool
		socketCleaned atomic.Bool
	)
	ops := daemonLifecycleOps{
		readPID:         func() (int, error) { return oldPID, nil },
		processRunning:  func(int) bool { return false },
		shutdownRPC:     func(context.Context) error { return nil },
		terminate:       func(int) error { return nil },
		cleanupPID:      func() { pidCleaned.Store(true) },
		cleanupSocket:   func() { socketCleaned.Store(true) },
		startBackground: func(bool) error { return nil },
		pollInterval:    time.Millisecond,
		gracePeriod:     time.Second,
		fallbackPeriod:  time.Second,
	}

	replaced, err := stopContextWithOps(t.Context(), ops)
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("stop reported a replacement for unchanged stale ownership")
	}
	if !pidCleaned.Load() || !socketCleaned.Load() {
		t.Fatalf("stale cleanup = pid:%t socket:%t, want both", pidCleaned.Load(), socketCleaned.Load())
	}
}

func TestStopContextJoinsRPCAndFallbackErrors(t *testing.T) {
	const oldPID = 63
	rpcErr := errors.New("rpc shutdown sentinel")
	terminateErr := errors.New("terminate sentinel")
	ops := daemonLifecycleOps{
		readPID:         func() (int, error) { return oldPID, nil },
		processRunning:  func(int) bool { return true },
		shutdownRPC:     func(context.Context) error { return rpcErr },
		terminate:       func(int) error { return terminateErr },
		cleanupPID:      func() {},
		cleanupSocket:   func() {},
		startBackground: func(bool) error { return nil },
		pollInterval:    time.Millisecond,
		gracePeriod:     time.Millisecond,
		fallbackPeriod:  time.Millisecond,
	}

	_, err := stopContextWithOps(t.Context(), ops)
	if !errors.Is(err, rpcErr) || !errors.Is(err, terminateErr) {
		t.Fatalf("stop error = %v, want joined RPC and terminate errors", err)
	}
}

func TestRestartPreservesConcurrentLiveReplacement(t *testing.T) {
	const (
		oldPID = 74
		newPID = 75
	)
	var (
		readCalls atomic.Int32
		cleaned   atomic.Bool
		started   atomic.Bool
	)
	ops := daemonLifecycleOps{
		readPID: func() (int, error) {
			if readCalls.Add(1) == 1 {
				return oldPID, nil
			}
			return newPID, nil
		},
		processRunning: func(pid int) bool { return pid == newPID },
		shutdownRPC:    func(context.Context) error { return nil },
		terminate:      func(int) error { return nil },
		cleanupPID:     func() { cleaned.Store(true) },
		cleanupSocket:  func() { cleaned.Store(true) },
		startBackground: func(bool) error {
			started.Store(true)
			return nil
		},
		pollInterval:   time.Millisecond,
		gracePeriod:    time.Second,
		fallbackPeriod: time.Second,
	}

	if err := restartWithOps(t.Context(), false, ops); err != nil {
		t.Fatal(err)
	}
	if cleaned.Load() {
		t.Fatal("restart cleaned files owned by a live replacement")
	}
	if started.Load() {
		t.Fatal("restart launched a second replacement")
	}
}

func TestRestartCancellationDoesNotStartReplacement(t *testing.T) {
	const oldPID = 86
	started := atomic.Bool{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ops := daemonLifecycleOps{
		readPID:         func() (int, error) { return oldPID, nil },
		processRunning:  func(int) bool { return true },
		shutdownRPC:     func(context.Context) error { return nil },
		terminate:       func(int) error { return nil },
		cleanupPID:      func() {},
		cleanupSocket:   func() {},
		startBackground: func(bool) error { started.Store(true); return nil },
		pollInterval:    time.Millisecond,
		gracePeriod:     time.Second,
		fallbackPeriod:  time.Second,
	}

	err := restartWithOps(ctx, false, ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("restart error = %v, want context canceled", err)
	}
	if started.Load() {
		t.Fatal("restart started replacement after cancellation")
	}
}
