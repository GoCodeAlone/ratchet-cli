package commands

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

// compactCmd triggers manual context compression for the current session.
// It sets TriggerCompact on the result so the chat view can call CompactSession
// on the daemon using the session ID it already holds.
func compactCmd(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	return &Result{
		Lines:          []string{"Compressing conversation context…"},
		TriggerCompact: true,
	}
}

// reviewCmd runs the built-in code-reviewer agent on the current git diff.
func reviewCmd(c *client.Client) *Result {
	diff, err := gitDiff()
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error getting git diff: %v", err)}}
	}
	if diff == "" {
		return &Result{Lines: []string{"No uncommitted changes to review."}}
	}
	lines := []string{
		"Starting code review on current git diff...",
		"",
		"Diff summary:",
	}
	// Show a trimmed preview of the diff
	diffLines := strings.Split(diff, "\n")
	preview := diffLines
	if len(preview) > 20 {
		preview = diffLines[:20]
		preview = append(preview, fmt.Sprintf("... (%d more lines)", len(diffLines)-20))
	}
	lines = append(lines, preview...)
	lines = append(lines,
		"",
		"Use the code-reviewer agent via /agents to see full review results.",
	)
	return &Result{Lines: lines}
}

func gitDiff() (string, error) {
	out, err := exec.Command("git", "diff", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
