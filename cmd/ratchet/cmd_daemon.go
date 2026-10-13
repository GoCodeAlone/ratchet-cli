package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
		debug := false
		for _, a := range args[1:] {
			if a == "--background" || a == "-b" {
				bg = true
			}
			if a == "--debug" {
				debug = true
			}
		}
		if bg {
			if err := daemon.StartBackground(debug); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("daemon started in background")
		} else {
			if err := daemon.Start(context.Background(), debug); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	case "stop":
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := daemon.StopContext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("daemon stopped")
	case "restart":
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := daemon.Restart(ctx, false); err != nil {
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
