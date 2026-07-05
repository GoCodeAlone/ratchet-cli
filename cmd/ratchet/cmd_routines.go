package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/routines"
)

func handleRoutines(args []string) {
	if err := executeRoutines(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func executeRoutines(args []string, w io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(w, "Usage: ratchet routines <add|list|show|run|pause|resume|remove>")
		return nil
	}
	store, err := routines.LoadDefault()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("routines add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		schedule := fs.String("schedule", "", "schedule")
		prompt := fs.String("prompt", "", "prompt")
		cwd := fs.String("cwd", "", "working directory")
		provider := fs.String("provider", "", "provider")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *prompt == "" && fs.NArg() > 0 {
			*prompt = strings.Join(fs.Args(), " ")
		}
		def, err := store.Add(routines.AddRequest{Schedule: *schedule, Prompt: *prompt, CWD: expandHome(*cwd), Provider: *provider})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Added routine: %s\n", def.ID)
	case "list":
		defs := store.List()
		if len(defs) == 0 {
			fmt.Fprintln(w, "No routines configured.")
			return nil
		}
		fmt.Fprintf(w, "%-38s %-10s %-10s %s\n", "ID", "SCHEDULE", "STATUS", "PROMPT")
		for _, def := range defs {
			status := "active"
			if def.Paused {
				status = "paused"
			}
			fmt.Fprintf(w, "%-38s %-10s %-10s %s\n", def.ID, def.Schedule, status, truncateRoutineText(def.Prompt, 48))
		}
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet routines show <id>")
		}
		def, ok := store.Get(args[1])
		if !ok {
			return fmt.Errorf("routine %q not found", args[1])
		}
		printRoutine(w, def)
		runs := store.RunsForRoutine(def.ID)
		if len(runs) > 0 {
			sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
			fmt.Fprintf(w, "runs: %d\nlastRun: %s %s\n", len(runs), runs[0].ID, runs[0].Status)
		}
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet routines run <id>")
		}
		run, err := store.RunManual(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Recorded routine run: %s\n", run.ID)
	case "pause", "resume":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet routines %s <id>", args[0])
		}
		if args[0] == "pause" {
			err = store.Pause(args[1])
		} else {
			err = store.Resume(args[1])
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Routine %s %sd.\n", args[1], args[0])
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet routines remove <id>")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removed routine: %s\n", args[1])
	default:
		return fmt.Errorf("unknown routines command: %s", args[0])
	}
	return nil
}

func printRoutine(w io.Writer, def routines.Definition) {
	status := "active"
	if def.Paused {
		status = "paused"
	}
	fmt.Fprintf(w, "id: %s\nschedule: %s\nstatus: %s\nprompt: %s\n", def.ID, def.Schedule, status, def.Prompt)
	if def.CWD != "" {
		fmt.Fprintf(w, "cwd: %s\n", def.CWD)
	}
	if def.Provider != "" {
		fmt.Fprintf(w, "provider: %s\n", def.Provider)
	}
}

func truncateRoutineText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "."
}
