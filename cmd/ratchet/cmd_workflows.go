package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/workflows"
)

func handleWorkflows(args []string) {
	if err := executeWorkflows(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func executeWorkflows(args []string, w io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(w, "Usage: ratchet workflows <install|list|show|run|stop|resume>")
		return nil
	}
	store, err := workflows.LoadDefault()
	if err != nil {
		return err
	}
	switch args[0] {
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet workflows install <file>")
		}
		def, err := store.InstallFile(expandHome(args[1]))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Installed workflow: %s\n", def.Name)
	case "list":
		defs := store.List()
		if len(defs) == 0 {
			fmt.Fprintln(w, "No workflows installed.")
			return nil
		}
		fmt.Fprintf(w, "%-24s %-6s %-6s %s\n", "NAME", "NODES", "EDGES", "DESCRIPTION")
		for _, def := range defs {
			fmt.Fprintf(w, "%-24s %-6d %-6d %s\n", def.Name, len(def.Nodes), len(def.Edges), def.Description)
		}
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet workflows show <name>")
		}
		def, ok := store.Get(args[1])
		if !ok {
			return fmt.Errorf("workflow %q not found", args[1])
		}
		printWorkflow(w, def)
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet workflows run <name|file>")
		}
		target := expandHome(args[1])
		name := args[1]
		if workflowPathExists(target) {
			def, err := store.InstallFile(target)
			if err != nil {
				return err
			}
			name = def.Name
		}
		run, err := store.Run(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Recorded workflow run: %s\n", run.ID)
	case "stop":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet workflows stop <run-id>")
		}
		if err := store.Stop(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(w, "Stopped workflow run: %s\n", args[1])
	case "resume":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet workflows resume <run-id>")
		}
		run, err := store.Resume(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Resumed workflow run: %s\n", run.ID)
	default:
		return fmt.Errorf("unknown workflows command: %s", args[0])
	}
	return nil
}

func printWorkflow(w io.Writer, def workflows.Definition) {
	fmt.Fprintf(w, "name: %s\nnodes: %d\nedges: %d\n", def.Name, len(def.Nodes), len(def.Edges))
	if def.Description != "" {
		fmt.Fprintf(w, "description: %s\n", def.Description)
	}
	if def.Source != "" {
		fmt.Fprintf(w, "source: %s\n", def.Source)
	}
}

func workflowPathExists(path string) bool {
	if path == "" || strings.Contains(path, "\x00") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
