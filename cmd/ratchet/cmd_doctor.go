package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	"github.com/GoCodeAlone/ratchet-cli/internal/doctor"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

func handleDoctor(args []string) {
	if err := runDoctor(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "doctor error: %v\n", err)
		exitProcess(1)
	}
}

func runDoctor(args []string, w io.Writer) error {
	var jsonOut bool
	fs := flag.NewFlagSet("ratchet doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: ratchet doctor [--json]")
	}
	daemonStatus, err := daemon.Status()
	if err != nil {
		daemonStatus = err.Error()
	}
	report := doctor.Collect(version.Version, version.Commit, version.Date, daemonStatus)
	if jsonOut {
		return json.NewEncoder(w).Encode(report)
	}
	_, err = fmt.Fprintln(w, strings.Join(report.TextLines(), "\n"))
	return err
}
