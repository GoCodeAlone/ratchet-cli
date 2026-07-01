package retro

import (
	"slices"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

func TestRouteFindingsLocalConfigDisabledEmitsInstruction(t *testing.T) {
	report := Report{Findings: []Finding{{
		Pattern:     "permission denial",
		Evidence:    "bash blocked",
		Project:     ProjectLocalConfig,
		LocalAction: "Allow the command in local trust config.",
	}}}

	routed := RouteFindings(config.RetroConfig{
		Enabled:              true,
		LocalChanges:         false,
		UpstreamInstructions: true,
	}, report)

	if len(routed.LocalActions) != 0 {
		t.Fatalf("local actions = %#v, want none", routed.LocalActions)
	}
	if !slices.ContainsFunc(routed.UpstreamInstructions, func(s string) bool {
		return strings.Contains(s, "local changes are disabled") && strings.Contains(s, "Allow the command")
	}) {
		t.Fatalf("upstream instructions = %#v", routed.UpstreamInstructions)
	}
}

func TestRouteFindingsLocalConfigEnabledEmitsLocalAction(t *testing.T) {
	report := Report{Findings: []Finding{{
		Pattern:     "permission denial",
		Project:     ProjectLocalConfig,
		LocalAction: "Allow the command in local trust config.",
	}}}

	routed := RouteFindings(config.RetroConfig{
		Enabled:              true,
		LocalChanges:         true,
		UpstreamInstructions: true,
	}, report)

	if len(routed.LocalActions) != 1 {
		t.Fatalf("local actions = %#v", routed.LocalActions)
	}
	if len(routed.UpstreamInstructions) != 0 {
		t.Fatalf("upstream instructions = %#v, want none", routed.UpstreamInstructions)
	}
}

func TestRouteFindingsRatchetAndThirdPartyInstructions(t *testing.T) {
	report := Report{Findings: []Finding{
		{
			Pattern:        "test failure",
			Project:        ProjectRatchetCLI,
			UpstreamAction: "submit a ratchet-cli PR with regression coverage.",
		},
		{
			Pattern:        "external harness gap",
			Project:        "zed",
			UpstreamAction: "file an upstream Zed issue with reproduction steps.",
		},
	}}

	routed := RouteFindings(config.RetroConfig{
		Enabled:              true,
		UpstreamInstructions: true,
	}, report)

	if len(routed.UpstreamInstructions) != 2 {
		t.Fatalf("upstream instructions = %#v", routed.UpstreamInstructions)
	}
	if !strings.Contains(routed.UpstreamInstructions[0], "ratchet-cli PR") {
		t.Fatalf("ratchet instruction = %q", routed.UpstreamInstructions[0])
	}
	if !strings.Contains(routed.UpstreamInstructions[1], "third-party") {
		t.Fatalf("third-party instruction = %q", routed.UpstreamInstructions[1])
	}
}
