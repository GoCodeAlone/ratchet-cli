//go:build windows

package daemon

import (
	"os"
	"syscall"
)

var reloadSignal os.Signal = os.Interrupt

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
	return &syscall.SysProcAttr{}
}
