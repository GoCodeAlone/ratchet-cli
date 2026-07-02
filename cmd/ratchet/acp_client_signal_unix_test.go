//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestACPClientWatchSignalsIncludeInterruptAndTerminate(t *testing.T) {
	signals := acpClientWatchSignals()
	if !hasSignal(signals, os.Interrupt) {
		t.Fatalf("signals = %#v, want os.Interrupt", signals)
	}
	if !hasSignal(signals, syscall.SIGTERM) {
		t.Fatalf("signals = %#v, want SIGTERM", signals)
	}
}

func hasSignal(signals []os.Signal, want os.Signal) bool {
	for _, sig := range signals {
		if sig == want {
			return true
		}
	}
	return false
}
