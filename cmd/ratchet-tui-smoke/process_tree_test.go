package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	processTreeRoleEnv   = "RATCHET_PROCESS_TREE_TEST_ROLE"
	processTreeRootEnv   = "RATCHET_PROCESS_TREE_TEST_ROOT"
	processTreeSecretEnv = "RATCHET_PROCESS_TREE_TEST_SECRET"
)

func runBoundedCommand(
	root string,
	env []string,
	executionLimit time.Duration,
	reapLimit time.Duration,
	name string,
	args ...string,
) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	configureProcessTree(cmd)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return output.String(), fmt.Errorf("start command %s: %w", command, err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	executionCtx, cancelExecution := context.WithTimeout(context.Background(), executionLimit)
	defer cancelExecution()
	select {
	case err := <-waitDone:
		if err != nil {
			return output.String(), fmt.Errorf("command %s: %w", command, err)
		}
		return output.String(), nil
	case <-executionCtx.Done():
	}

	timeoutErr := fmt.Errorf(
		"command %s exceeded %s: %w",
		command,
		executionLimit,
		executionCtx.Err(),
	)
	reapCtx, cancelReap := context.WithTimeout(context.Background(), reapLimit)
	defer cancelReap()
	treeErr := terminateProcessTree(reapCtx, cmd.Process.Pid)

	select {
	case <-waitDone:
		return output.String(), errors.Join(timeoutErr, treeErr)
	case <-reapCtx.Done():
		rootErr := cmd.Process.Kill()
		return output.String(), errors.Join(
			timeoutErr,
			treeErr,
			fmt.Errorf("reap command %s: %w", command, reapCtx.Err()),
			rootErr,
		)
	}
}

func TestBoundedCommandKillsProcessTree(t *testing.T) {
	root := t.TempDir()
	secret := "process-tree-secret-must-not-leak"
	start := time.Now()
	output, err := runBoundedCommand(
		"",
		[]string{
			processTreeRoleEnv + "=parent",
			processTreeRootEnv + "=" + root,
			processTreeSecretEnv + "=" + secret,
		},
		750*time.Millisecond,
		5*time.Second,
		os.Args[0],
		"-test.run=^TestProcessTreeFixture$",
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded command error = %v, want deadline exceeded\n%s", err, output)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("bounded command returned after %s, want at most 8s", elapsed)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("bounded command error leaked environment secret: %v", err)
	}
	if !strings.Contains(err.Error(), "-test.run=^TestProcessTreeFixture$") {
		t.Fatalf("bounded command error omitted command arguments: %v", err)
	}

	pids := make([]int, 0, 2)
	for _, name := range []string{"parent.pid", "child.pid"} {
		pids = append(pids, readProcessTreePID(t, filepath.Join(root, name)))
	}
	t.Cleanup(func() {
		for _, pid := range pids {
			process, err := os.FindProcess(pid)
			if err == nil {
				_ = process.Kill()
			}
		}
	})
	for _, pid := range pids {
		waitForProcessTreeExit(t, pid)
	}
}

func TestBoundedCommandRedactsEnvironment(t *testing.T) {
	secret := "redaction-sentinel-must-not-leak"
	output, err := runBoundedCommand(
		"",
		[]string{
			processTreeRoleEnv + "=block",
			processTreeSecretEnv + "=" + secret,
		},
		50*time.Millisecond,
		time.Second,
		os.Args[0],
		"-test.run=^TestProcessTreeFixture$",
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded command error = %v, want deadline exceeded\n%s", err, output)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("bounded command error leaked environment secret: %v", err)
	}
	if !strings.Contains(err.Error(), "-test.run=^TestProcessTreeFixture$") {
		t.Fatalf("bounded command error omitted command arguments: %v", err)
	}
}

func TestProcessTreeFixture(t *testing.T) {
	switch os.Getenv(processTreeRoleEnv) {
	case "":
		return
	case "block":
		for {
			time.Sleep(time.Hour)
		}
	case "child":
		writeProcessTreePID(t, "child.pid")
		for {
			time.Sleep(time.Hour)
		}
	case "parent":
		writeProcessTreePID(t, "parent.pid")
		cmd := exec.Command(os.Args[0], "-test.run=^TestProcessTreeFixture$")
		cmd.Env = append(os.Environ(), processTreeRoleEnv+"=child")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child fixture: %v", err)
		}
		waitForProcessTreeFile(t, filepath.Join(os.Getenv(processTreeRootEnv), "child.pid"))
		for {
			time.Sleep(time.Hour)
		}
	default:
		t.Fatalf("unknown process tree fixture role %q", os.Getenv(processTreeRoleEnv))
	}
}

func writeProcessTreePID(t *testing.T, name string) {
	t.Helper()
	root := os.Getenv(processTreeRootEnv)
	if root == "" {
		t.Fatal("process tree fixture root is empty")
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func waitForProcessTreeFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func readProcessTreePID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture PID %s: %v", path, err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("parse fixture PID %s: %v", path, err)
	}
	return pid
}

func waitForProcessTreeExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processTreeTestRunning(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fixture process %d remains alive", pid)
}
