//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func terminateProcessTree(ctx context.Context, pid int) error {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return errors.Join(
			fmt.Errorf("resolve Windows system directory: %w", err),
			killRootProcess(pid),
		)
	}

	taskkill := filepath.Join(systemDir, "taskkill.exe")
	cmd := exec.CommandContext(
		ctx,
		taskkill,
		"/T",
		"/F",
		"/PID",
		strconv.Itoa(pid),
	)
	output, taskkillErr := cmd.CombinedOutput()
	if taskkillErr == nil {
		return nil
	}
	return errors.Join(
		fmt.Errorf(
			"terminate process tree %d with taskkill: %w: %s",
			pid,
			taskkillErr,
			strings.TrimSpace(string(output)),
		),
		killRootProcess(pid),
	)
}

func killRootProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find root process %d: %w", pid, err)
	}
	if err := process.Kill(); err != nil {
		return fmt.Errorf("kill root process %d: %w", pid, err)
	}
	return errors.New("root process killed without proof that descendants exited")
}

func processTreeTestRunning(pid int) bool {
	const stillActive = 259

	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() {
		_ = windows.CloseHandle(handle)
	}()

	var exitCode uint32
	return windows.GetExitCodeProcess(handle, &exitCode) == nil && exitCode == stillActive
}
