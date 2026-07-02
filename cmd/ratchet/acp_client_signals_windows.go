//go:build windows

package main

import "os"

func acpClientWatchSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
