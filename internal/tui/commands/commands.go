package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// Result holds the output of a parsed slash command.
type Result struct {
	Lines                []string
	NavigateToOnboarding bool
	Quit                 bool
	ClearChat            bool
	TriggerCompact       bool // ask the caller to compress the current session's context
}

// Parse checks if input is a slash command and executes it.
// Returns nil if input is not a command.
func Parse(input string, c *client.Client) *Result {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}

	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		return helpCmd()
	case "/model":
		return modelCmd(parts[1:], c)
	case "/clear":
		return &Result{
			Lines:     []string{"Conversation cleared."},
			ClearChat: true,
		}
	case "/cost":
		return costCmd(parts[1:], c)
	case "/agents":
		return agentsCmd(c)
	case "/sessions":
		return sessionsCmd(c)
	case "/exit":
		return &Result{
			Lines: []string{"Goodbye!"},
			Quit:  true,
		}
	case "/provider":
		if len(parts) < 2 {
			return &Result{Lines: []string{
				"Usage: /provider <list|add|remove|default|test> [alias]",
			}}
		}
		return providerCmd(parts[1:], c)
	case "/loop":
		if len(parts) < 3 {
			return &Result{Lines: []string{"Usage: /loop <interval> <command>"}}
		}
		return cronCreate(parts[1], strings.Join(parts[2:], " "), c)
	case "/cron":
		if len(parts) < 2 {
			return &Result{Lines: []string{
				"Usage: /cron <expr> <command> | /cron list | /cron pause <id> | /cron resume <id> | /cron stop <id>",
			}}
		}
		return cronCmd(parts[1:], c)
	case "/fleet":
		if len(parts) < 2 {
			return &Result{Lines: []string{"Usage: /fleet <plan_id> [max_workers]"}}
		}
		return fleetCmd(parts[1:], c)
	case "/mcp":
		if len(parts) < 2 {
			return &Result{Lines: []string{"Usage: /mcp <list|enable <name>|disable <name>>"}}
		}
		return mcpCmd(parts[1:])
	case "/compact":
		return compactCmd(c)
	case "/review":
		return reviewCmd(c)
	case "/team":
		return teamCmd(parts[1:], c)
	case "/plan":
		return &Result{Lines: []string{"Plan mode: wait for the assistant to propose a plan, then use /approve or /reject."}}
	case "/approve":
		if len(parts) < 2 {
			return &Result{Lines: []string{"Usage: /approve <plan_id> [skip_step_id ...]"}}
		}
		return approvePlanCmd(parts[1], parts[2:], c)
	case "/reject":
		if len(parts) < 2 {
			return &Result{Lines: []string{"Usage: /reject <plan_id> [feedback]"}}
		}
		return rejectPlanCmd(parts[1], strings.Join(parts[2:], " "), c)
	case "/jobs":
		return jobsCmd(c)
	default:
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown command: %s — type /help for available commands", cmd),
		}}
	}
}

func helpCmd() *Result {
	return &Result{Lines: []string{
		"Available commands:",
		"  /help                      Show this help",
		"  /model                     Show current model",
		"  /clear                     Clear conversation",
		"  /cost                      Show token usage",
		"  /agents                    List active agents",
		"  /sessions                  List sessions",
		"  /provider list             List configured providers",
		"  /provider add              Add a new provider (opens wizard)",
		"  /provider remove <alias>   Remove a provider",
		"  /provider default <alias>  Set default provider",
		"  /provider test <alias>     Test provider connection",
		"  /fleet <plan_id>           Start fleet execution for a plan",
		"  /team status <id>          Get team status",
		"  /team start <task>         Start a new team for a task",
		"  /plan                      Show plan mode info",
		"  /approve <plan_id>         Approve a proposed plan",
		"  /reject <plan_id>          Reject a proposed plan",
		"  /loop <interval> <cmd>     Schedule a recurring command (e.g. /loop 5m /review)",
		"  /cron <expr> <cmd>         Schedule with cron expression (e.g. /cron */10 * * * * /digest)",
		"  /cron list                 List all cron jobs",
		"  /cron pause <id>           Pause a cron job",
		"  /cron resume <id>          Resume a paused cron job",
		"  /cron stop <id>            Stop and remove a cron job",
		"  /jobs                      Show unified job control panel (or use Ctrl+J)",
		"  /compact                   Manually compress conversation context",
		"  /review                    Run built-in code-reviewer on current git diff",
		"  /exit                      Quit ratchet",
	}}
}

func providerCmd(args []string, c *client.Client) *Result {
	sub := strings.ToLower(args[0])
	switch sub {
	case "list":
		return providerList(c)
	case "add":
		return &Result{
			Lines:                []string{"Opening provider setup wizard..."},
			NavigateToOnboarding: true,
		}
	case "remove":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider remove <alias>"}}
		}
		return providerRemove(args[1], c)
	case "default":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider default <alias>"}}
		}
		return providerDefault(args[1], c)
	case "test":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider test <alias>"}}
		}
		return providerTest(args[1], c)
	default:
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown provider command: %s", sub),
			"Available: list, add, remove, default, test",
		}}
	}
}

func providerList(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListProviders(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	if len(resp.Providers) == 0 {
		return &Result{Lines: []string{"No providers configured. Use /provider add to set one up."}}
	}
	lines := []string{"Configured providers:", ""}
	for _, p := range resp.Providers {
		def := ""
		if p.IsDefault {
			def = " (default)"
		}
		lines = append(lines, fmt.Sprintf("  %-12s %-10s model=%s%s", p.Alias, p.Type, p.Model, def))
	}
	return &Result{Lines: lines}
}

func providerRemove(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.RemoveProvider(context.Background(), alias); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error removing %s: %v", alias, err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Provider %q removed.", alias)}}
}

func providerDefault(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.SetDefaultProvider(context.Background(), alias); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error setting default: %v", err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Provider %q set as default.", alias)}}
}

func providerTest(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	result, err := c.TestProvider(context.Background(), alias)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error testing %s: %v", alias, err)}}
	}
	if result.Success {
		return &Result{Lines: []string{
			fmt.Sprintf("Provider %q: OK (%dms)", alias, result.LatencyMs),
		}}
	}
	return &Result{Lines: []string{
		fmt.Sprintf("Provider %q: FAILED — %s", alias, result.Message),
	}}
}

func modelCmd(args []string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListProviders(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}

	if len(args) == 0 {
		lines := []string{"Current providers and models:", ""}
		for _, p := range resp.Providers {
			marker := "  "
			if p.IsDefault {
				marker = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%-12s %s", marker, p.Alias, p.Model))
		}
		lines = append(lines, "", "Use /model <alias> <model-name> to change a provider's model.")
		return &Result{Lines: lines}
	}

	if len(args) == 1 {
		return &Result{Lines: []string{
			"To switch model, use: /model <alias> <model-name>",
			"Use /model to see available providers and their current models.",
		}}
	}

	return &Result{Lines: []string{
		"Model switching requires daemon support (not yet implemented).",
		"For now, use /provider remove + /provider add to change models.",
	}}
}

func agentsCmd(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListAgents(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	if len(resp.Agents) == 0 {
		return &Result{Lines: []string{"No active agents."}}
	}
	lines := []string{"Active agents:", ""}
	for _, a := range resp.Agents {
		lines = append(lines, fmt.Sprintf("  %-20s %-10s %s", a.Name, a.Status, a.Role))
	}
	return &Result{Lines: lines}
}

func sessionsCmd(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListSessions(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	if len(resp.Sessions) == 0 {
		return &Result{Lines: []string{"No sessions."}}
	}
	lines := []string{"Sessions:", ""}
	for _, s := range resp.Sessions {
		id := s.Id
		if len(id) > 8 {
			id = id[:8]
		}
		lines = append(lines, fmt.Sprintf("  %-10s %-10s %s", id, s.Status, s.Name))
	}
	return &Result{Lines: lines}
}

// cronCreate creates a new cron job with a duration-style schedule (used by /loop).
func cronCreate(schedule, command string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	job, err := c.CreateCron(context.Background(), "", schedule, command)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error creating cron job: %v", err)}}
	}
	id := job.Id
	if len(id) > 8 {
		id = id[:8]
	}
	return &Result{Lines: []string{
		fmt.Sprintf("Cron job created: %s  schedule=%s  command=%s", id, job.Schedule, job.Command),
	}}
}

func cronCmd(args []string, c *client.Client) *Result {
	sub := strings.ToLower(args[0])
	switch sub {
	case "list":
		return cronList(c)
	case "pause":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /cron pause <id>"}}
		}
		return cronPause(args[1], c)
	case "resume":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /cron resume <id>"}}
		}
		return cronResume(args[1], c)
	case "stop":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /cron stop <id>"}}
		}
		return cronStop(args[1], c)
	default:
		// Treat first arg as start of cron expression; remaining as command.
		// "/cron */10 * * * * /digest" → expr="*/10 * * * *", cmd="/digest"
		if len(args) < 6 {
			return &Result{Lines: []string{
				"Usage: /cron <expr> <command>  (5-field cron expression followed by command)",
				"       /cron list | pause <id> | resume <id> | stop <id>",
			}}
		}
		expr := strings.Join(args[:5], " ")
		cmd := strings.Join(args[5:], " ")
		return cronCreate(expr, cmd, c)
	}
}

func cronList(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListCrons(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	if len(resp.Jobs) == 0 {
		return &Result{Lines: []string{"No cron jobs scheduled."}}
	}
	lines := []string{"Cron jobs:", ""}
	for _, j := range resp.Jobs {
		id := j.Id
		if len(id) > 8 {
			id = id[:8]
		}
		lines = append(lines, fmt.Sprintf("  %-10s %-8s %-20s %s", id, j.Status, j.Schedule, j.Command))
	}
	return &Result{Lines: lines}
}

func cronPause(id string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.PauseCron(context.Background(), id); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error pausing %s: %v", id, err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Cron job %s paused.", id)}}
}

func cronResume(id string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.ResumeCron(context.Background(), id); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error resuming %s: %v", id, err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Cron job %s resumed.", id)}}
}

func cronStop(id string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.StopCron(context.Background(), id); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error stopping %s: %v", id, err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Cron job %s stopped.", id)}}
}

// teamCmd handles /team subcommands.
func teamCmd(args []string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage: /team status <team_id> | /team start <task description>",
		}}
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "status":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /team status <team_id>"}}
		}
		return teamStatus(args[1], c)
	case "start":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /team start <task description>"}}
		}
		task := strings.Join(args[1:], " ")
		return teamStart(task, c)
	default:
		return &Result{Lines: []string{fmt.Sprintf("Unknown team subcommand: %s", sub)}}
	}
}

func teamStatus(teamID string, c *client.Client) *Result {
	st, err := c.GetTeamStatus(context.Background(), teamID)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	lines := []string{fmt.Sprintf("Team %s (%s):", teamID[:min(8, len(teamID))], st.Status), ""}
	for _, a := range st.Agents {
		lines = append(lines, fmt.Sprintf("  %-20s %-12s %-10s %s", a.Name, a.Role, a.Status, a.Model))
	}
	return &Result{Lines: lines}
}

func teamStart(task string, c *client.Client) *Result {
	go func() {
		// Fire-and-forget: start team async.
		_, _ = c.StartTeam(context.Background(), &pb.StartTeamReq{Task: task})
	}()
	return &Result{Lines: []string{
		fmt.Sprintf("Starting team for task: %s", task),
		"Team events will appear in the chat stream.",
	}}
}

// costCmd shows token usage and, when a fleet ID is provided, a per-worker
// model/cost breakdown based on the fleet's worker assignments.
func costCmd(args []string, c *client.Client) *Result {
	if len(args) > 0 && c != nil {
		fleetID := args[0]
		fs, err := c.GetFleetStatus(context.Background(), fleetID)
		if err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error fetching fleet %s: %v", fleetID, err)}}
		}
		lines := []string{
			fmt.Sprintf("Fleet %s — per-worker model breakdown:", fleetID[:min(8, len(fleetID))]),
			fmt.Sprintf("  %-20s %-30s %-15s %s", "Worker", "Step", "Model", "Status"),
			strings.Repeat("─", 70),
		}
		for _, w := range fs.Workers {
			lines = append(lines, fmt.Sprintf("  %-20s %-30s %-15s %s",
				w.Name, w.StepId, w.Model, w.Status))
		}
		lines = append(lines, fmt.Sprintf("\nTotal workers: %d  Completed: %d/%d",
			len(fs.Workers), fs.Completed, fs.Total))
		return &Result{Lines: lines}
	}
	return &Result{Lines: []string{
		"Token usage is shown in the status bar below the input.",
		"For per-worker breakdown: /cost <fleet_id>",
	}}
}

// mcpCmd handles /mcp subcommands. MCP discovery runs on the daemon side;
// these commands query available CLIs and enable/disable them via the discoverer.
func mcpCmd(args []string) *Result {
	sub := strings.ToLower(args[0])
	switch sub {
	case "list":
		available := mcp.AvailableCLIs()
		known := mcp.KnownCLINames()
		lines := []string{"MCP CLI tools (discovered from PATH):"}
		for _, name := range known {
			if tools, ok := available[name]; ok {
				lines = append(lines, fmt.Sprintf("  %-8s [installed]  tools: %s", name, strings.Join(tools, ", ")))
			} else {
				lines = append(lines, fmt.Sprintf("  %-8s [not found]", name))
			}
		}
		lines = append(lines, "", "Use /mcp enable <cli> or /mcp disable <cli> to manage registration.")
		return &Result{Lines: lines}
	case "enable":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /mcp enable <cli-name>"}}
		}
		available := mcp.AvailableCLIs()
		cliName := args[1]
		if tools, ok := available[cliName]; ok {
			return &Result{Lines: []string{
				fmt.Sprintf("MCP CLI %q is installed with tools: %s", cliName, strings.Join(tools, ", ")),
				"The daemon will register it automatically on next startup.",
			}}
		}
		return &Result{Lines: []string{fmt.Sprintf("MCP CLI %q is not installed or not a known CLI.", cliName)}}
	case "disable":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /mcp disable <cli-name>"}}
		}
		return &Result{Lines: []string{
			fmt.Sprintf("MCP CLI %q will be excluded from discovery on next daemon startup.", args[1]),
			"Note: restart the daemon to apply the change.",
		}}
	default:
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown mcp subcommand: %s", sub),
			"Usage: /mcp <list|enable <name>|disable <name>>",
		}}
	}
}

