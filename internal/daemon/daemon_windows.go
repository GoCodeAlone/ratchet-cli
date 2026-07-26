//go:build windows

package daemon

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var reloadSignal os.Signal = os.Interrupt

func processRunning(pid int) bool {
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

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func terminateSignal() os.Signal {
	return os.Kill
}

func reloadSignalsSupported() bool {
	return false
}

func backgroundSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
