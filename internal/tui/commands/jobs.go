package commands

import (
	"context"
	"fmt"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

// jobsCmd handles the /jobs command — lists active jobs from the daemon.
func jobsCmd(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	list, err := c.ListJobs(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error listing jobs: %v", err)}}
	}
	if len(list.Jobs) == 0 {
		return &Result{Lines: []string{
			"No active jobs.",
			"Tip: use Ctrl+J to open the live job control panel.",
		}}
	}
	lines := []string{
		fmt.Sprintf("%-12s %-20s %-12s %-10s %s", "Type", "Name", "Status", "Elapsed", "ID"),
		fmt.Sprintf("%-12s %-20s %-12s %-10s %s", "----", "----", "------", "-------", "--"),
	}
	for _, j := range list.Jobs {
		elapsed := j.Elapsed
		if elapsed == "" {
			elapsed = "-"
		}
		lines = append(lines, fmt.Sprintf("%-12s %-20s %-12s %-10s %s",
			truncateStr(j.Type, 12),
			truncateStr(j.Name, 20),
			truncateStr(j.Status, 12),
			truncateStr(elapsed, 10),
			j.Id,
		))
	}
	lines = append(lines, "", "Tip: use Ctrl+J for the live job control panel.")
	return &Result{Lines: lines}
}

func truncateStr(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
