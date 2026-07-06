package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const policyMatrixSource = "docs/policy-matrix.md"

type policyMatrixRow struct {
	Layer  string `json:"layer"`
	Owner  string `json:"owner"`
	Status string `json:"status"`
	Rule   string `json:"rule"`
}

var policyMatrixRows = []policyMatrixRow{
	{
		Layer:  "Static config trust rules",
		Owner:  "internal/config plus workflow-plugin-agent/policy.TrustEngine",
		Status: "supported",
		Rule:   "Config provides daemon startup trust defaults; runtime and persistent changes do not rewrite config.",
	},
	{
		Layer:  "Runtime trust rules",
		Owner:  "daemon RPC, CLI, and TUI trust commands",
		Status: "supported",
		Rule:   "Mode and trust commands mutate or inspect daemon-local runtime state.",
	},
	{
		Layer:  "Persistent trust grants",
		Owner:  "workflow-plugin-agent/policy.PermissionStore through daemon RPC",
		Status: "supported",
		Rule:   "Durable allow and deny grants preserve deny-wins semantics.",
	},
	{
		Layer:  "Permission prompts",
		Owner:  "daemon permission gate and TUI prompt flow",
		Status: "supported",
		Rule:   "Unresolved decisions require explicit human approval instead of silent approval.",
	},
	{
		Layer:  "ACP client queue/drain",
		Owner:  "internal/acpclient",
		Status: "explicit-operator",
		Rule:   "Queued prompts execute only through operator-started watch or drain commands; background daemon drain is deferred.",
	},
	{
		Layer:  "ACP archive/compare/replay artifacts",
		Owner:  "internal/acpclient",
		Status: "supported",
		Rule:   "Local archives, raw event logs, compare bundles, and Go-native ACPX replay bundles are sensitive local artifacts.",
	},
	{
		Layer:  "ACP launch profiles",
		Owner:  "internal/acpclient profile store plus plugin templates",
		Status: "supported",
		Rule:   "Trusted local launch specs may be used by explicit foreground ACP commands.",
	},
	{
		Layer:  "Release artifact gates",
		Owner:  "GoReleaser, GitHub Actions, and internal/releaseguard",
		Status: "supported",
		Rule:   "Release archives, checksums, tap material, and CI release checks gate public release publication.",
	},
	{
		Layer:  "Sandbox/path/network controls",
		Owner:  "agent plugin trust logic, mesh path guard, and future sandbox work",
		Status: "partial",
		Rule:   "Implemented trust and path guards do not claim full sandbox, network, or per-tool escalation parity.",
	},
	{
		Layer:  "Hooks/extensions",
		Owner:  "internal/hooks, plugin manifests, plugin marketplaces, plugin reload, and future extension work",
		Status: "partial",
		Rule:   "Reviewable command hooks and plugin reload are supported; managed hooks, hidden autonomy, and SDK execution remain deferred.",
	},
	{
		Layer:  "Flow action nodes",
		Owner:  "internal/acpclient",
		Status: "supported",
		Rule:   "Flow action nodes run local commands only after explicit command-line grants.",
	},
	{
		Layer:  "Visible routines/workflows",
		Owner:  "internal/routines, internal/workflows, and CLI stores",
		Status: "supported",
		Rule:   "Local definitions and run records are visible state only; no hidden daemon worker is created.",
	},
	{
		Layer:  "Retro/self-improvement",
		Owner:  "internal/retro and local project evidence routing",
		Status: "partial",
		Rule:   "Retro analysis, handoff instructions, and bundles are opt-in and do not mutate config or open upstream PRs.",
	},
	{
		Layer:  "Blackboard notification-event export",
		Owner:  "daemon blackboard CLI plus workflow-plugin-messaging-core bridge helpers",
		Status: "supported",
		Rule:   "Local exports can be projected to Workflow messaging handoff records; external delivery belongs to Workflow plugins.",
	},
	{
		Layer:  "Per-agent/team scopes",
		Owner:  "daemon team manager and mesh configs",
		Status: "partial",
		Rule:   "Team messaging exists; per-agent permission scopes and channel routing need a future design.",
	},
	{
		Layer:  "Background drain",
		Owner:  "future design",
		Status: "deferred",
		Rule:   "Hidden background execution needs owner/session scope, cancellation, audit evidence, and redaction boundaries.",
	},
	{
		Layer:  "Managed hooks",
		Owner:  "future design",
		Status: "deferred",
		Rule:   "Managed hook distribution and local override behavior need a future policy decision.",
	},
	{
		Layer:  "Extension SDK",
		Owner:  "future design",
		Status: "deferred",
		Rule:   "SDK execution, marketplace update policy, mutation opt-in, and environment redaction need a future design.",
	},
	{
		Layer:  "Credentialed third-party agent CI",
		Owner:  "future design",
		Status: "deferred",
		Rule:   "Real provider matrices need secret handling, failure isolation, and artifact redaction.",
	},
	{
		Layer:  "Local-first gateway/channels",
		Owner:  "future design",
		Status: "deferred",
		Rule:   "Account/channel routing, inbox persistence, and non-main-session sandboxing need a future design.",
	},
}

func handlePolicy(args []string) {
	if err := runPolicy(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "policy error: %v\n", err)
		exitProcess(1)
	}
}

func runPolicy(args []string, w io.Writer) error {
	if len(args) == 0 {
		printPolicyUsage(w)
		return nil
	}
	switch args[0] {
	case "matrix":
		return runPolicyMatrix(args[1:], w)
	case "help", "--help", "-h":
		printPolicyUsage(w)
		return nil
	default:
		return fmt.Errorf("unknown policy command %q", args[0])
	}
}

func runPolicyMatrix(args []string, w io.Writer) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printPolicyMatrixUsage(w)
		return nil
	}
	var jsonOut bool
	var statusFilter string
	fs := flag.NewFlagSet("ratchet policy matrix", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	fs.StringVar(&statusFilter, "status", "", "filter by status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: ratchet policy matrix [--status status] [--json]")
	}
	rows, err := filterPolicyMatrixRows(statusFilter)
	if err != nil {
		return err
	}
	if jsonOut {
		payload := struct {
			Source string            `json:"source"`
			Status string            `json:"status,omitempty"`
			Rows   []policyMatrixRow `json:"rows"`
		}{
			Source: policyMatrixSource,
			Status: strings.TrimSpace(statusFilter),
			Rows:   rows,
		}
		return json.NewEncoder(w).Encode(payload)
	}
	fmt.Fprintln(w, "Ratchet policy matrix")
	fmt.Fprintf(w, "Source: %s\n\n", policyMatrixSource)
	fmt.Fprintf(w, "%-36s %-18s %s\n", "LAYER", "STATUS", "RULE")
	for _, row := range rows {
		fmt.Fprintf(w, "%-36s %-18s %s\n", row.Layer, row.Status, row.Rule)
	}
	return nil
}

func filterPolicyMatrixRows(status string) ([]policyMatrixRow, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return policyMatrixRows, nil
	}
	rows := make([]policyMatrixRow, 0)
	for _, row := range policyMatrixRows {
		if row.Status == status {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("unknown policy matrix status %q", status)
	}
	return rows, nil
}

func printPolicyUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: ratchet policy <command>

Commands:
  matrix [--status status] [--json]  Show supported, partial, explicit-operator, and deferred policy layers
`)
}

func printPolicyMatrixUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: ratchet policy matrix [--status status] [--json]

Show supported, partial, explicit-operator, and deferred policy layers from docs/policy-matrix.md.

Flags:
  --status status  Filter by supported, partial, explicit-operator, or deferred
  --json  Emit JSON
`)
}
