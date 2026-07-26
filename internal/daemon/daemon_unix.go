//go:build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// reloadSignal is the signal used to trigger graceful reload. Exported as a
// variable so tests can override it on platforms where SIGUSR1 is unavailable.
var reloadSignal os.Signal = syscall.SIGUSR1

func processRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}

func shutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

func terminateSignal() os.Signal {
	return syscall.SIGTERM
}

func reloadSignalsSupported() bool {
	return true
}

func backgroundSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
