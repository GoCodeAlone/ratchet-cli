package acpclient

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	profileProcessHelperEnv    = "RATCHET_PROFILE_PROCESS_HELPER"
	profileProcessModeEnv      = "RATCHET_PROFILE_PROCESS_MODE"
	profileProcessStorePathEnv = "RATCHET_PROFILE_PROCESS_STORE_PATH"
	profileProcessProfileEnv   = "RATCHET_PROFILE_PROCESS_PROFILE"
	profileProcessReadyPathEnv = "RATCHET_PROFILE_PROCESS_READY_PATH"
)

type profileProcess struct {
	cmd    *exec.Cmd
	done   chan error
	output bytes.Buffer
}

func TestProfileStoreProcessOperationsBlockOnProfileLock(t *testing.T) {
	for _, operation := range []string{"add", "trust", "remove", "list", "get"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profiles.json")
			store := NewProfileStore(path)
			if operation != "add" {
				if err := store.Add(Profile{Name: "fixture", Spec: AgentSpec{Command: "fixture-agent"}}); err != nil {
					t.Fatalf("seed Add: %v", err)
				}
			}
			release, err := acquireStoreFileLock(path + ".lock")
			if errors.Is(err, ErrStoreProcessLockUnsupported) {
				t.Skip("cross-process profile locks are unsupported on this platform")
			}
			if err != nil {
				t.Fatalf("hold profile lock: %v", err)
			}
			released := false
			defer func() {
				if !released {
					_ = release()
				}
			}()

			child := startProfileProcess(t, path, operation, "fixture")
			assertProfileProcessBlocked(t, child, operation)
			if err := release(); err != nil {
				t.Fatalf("release profile lock: %v", err)
			}
			released = true
			waitProfileProcess(t, child)
		})
	}
}

func TestProfileStoreProcessAddsBlockAndPreserveEveryUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	release, err := acquireStoreFileLock(path + ".lock")
	if errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Skip("cross-process profile locks are unsupported on this platform")
	}
	if err != nil {
		t.Fatalf("hold profile lock: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = release()
		}
	}()

	const processCount = 12
	children := make([]*profileProcess, 0, processCount)
	for i := range processCount {
		name := fmt.Sprintf("profile-%02d", i)
		children = append(children, startProfileProcess(t, path, "add", name))
	}
	for i, child := range children {
		assertProfileProcessBlocked(t, child, fmt.Sprintf("add %d", i))
	}
	if err := release(); err != nil {
		t.Fatalf("release profile lock: %v", err)
	}
	released = true
	for _, child := range children {
		waitProfileProcess(t, child)
	}

	profiles, err := NewProfileStore(path).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != processCount {
		t.Fatalf("profiles len = %d, want %d: %#v", len(profiles), processCount, profiles)
	}
}

func TestProfileStoreProcessHelper(t *testing.T) {
	if os.Getenv(profileProcessHelperEnv) != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv(profileProcessReadyPathEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	store := NewProfileStore(os.Getenv(profileProcessStorePathEnv))
	name := os.Getenv(profileProcessProfileEnv)
	var err error
	switch os.Getenv(profileProcessModeEnv) {
	case "add":
		err = store.Add(Profile{Name: name, Spec: AgentSpec{Command: "fixture-agent"}})
	case "trust":
		err = store.Trust(name)
	case "remove":
		err = store.Remove(name)
	case "list":
		_, err = store.List()
	case "get":
		_, err = store.Get(name)
	default:
		t.Fatalf("unknown profile process mode %q", os.Getenv(profileProcessModeEnv))
	}
	if err != nil {
		t.Fatalf("%s profile operation: %v", os.Getenv(profileProcessModeEnv), err)
	}
}

func startProfileProcess(t *testing.T, path, mode, profile string) *profileProcess {
	t.Helper()
	readyPath := filepath.Join(t.TempDir(), "ready")
	child := &profileProcess{done: make(chan error, 1)}
	child.cmd = exec.Command(os.Args[0], "-test.run=^TestProfileStoreProcessHelper$")
	child.cmd.Env = append(os.Environ(),
		profileProcessHelperEnv+"=1",
		profileProcessModeEnv+"="+mode,
		profileProcessStorePathEnv+"="+path,
		profileProcessProfileEnv+"="+profile,
		profileProcessReadyPathEnv+"="+readyPath,
	)
	child.cmd.Stdout = &child.output
	child.cmd.Stderr = &child.output
	if err := child.cmd.Start(); err != nil {
		t.Fatalf("start profile process: %v", err)
	}
	t.Cleanup(func() { _ = child.cmd.Process.Kill() })
	go func() { child.done <- child.cmd.Wait() }()
	waitForStoreLockHelperReady(t, readyPath)
	return child
}

func assertProfileProcessBlocked(t *testing.T, child *profileProcess, operation string) {
	t.Helper()
	select {
	case err := <-child.done:
		t.Fatalf("profile %s process exited while lock was held: %v\n%s", operation, err, child.output.String())
	case <-time.After(200 * time.Millisecond):
	}
}

func waitProfileProcess(t *testing.T, child *profileProcess) {
	t.Helper()
	select {
	case err := <-child.done:
		if err != nil {
			t.Fatalf("profile process failed: %v\n%s", err, child.output.String())
		}
	case <-time.After(acpClientProcessSmokeTimeout):
		killErr := child.cmd.Process.Kill()
		waitErr := <-child.done
		t.Fatalf("profile process remained blocked (kill: %v, wait: %v)\n%s", killErr, waitErr, child.output.String())
	}
}
