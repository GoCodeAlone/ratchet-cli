package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
)

func handleDaemon(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet daemon <start|stop|restart|status>")
		return
	}
	switch args[0] {
	case "start":
		bg := false
		for _, a := range args[1:] {
			if a == "--background" || a == "-b" {
				bg = true
			}
		}
		if bg {
			if err := daemon.StartBackground(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("daemon started in background")
		} else {
			if err := daemon.Start(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	case "stop":
		if err := daemon.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("daemon stopped")
	case "restart":
		// Stop the old daemon (ignore errors — it may not be running).
		_ = daemon.Stop()
		if err := daemon.StartBackground(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("daemon restarted")
	case "status":
		s, err := daemon.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(s)
	default:
		fmt.Printf("unknown daemon command: %s\n", args[0])
	}
}
