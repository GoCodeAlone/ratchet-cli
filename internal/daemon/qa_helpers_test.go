package daemon

// Test helpers for QA tests that alias types/functions from other packages
// to avoid import cycles while keeping tests in package daemon.

import (
	"context"
	"fmt"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// providerMessage is a local alias for provider.Message used in QA tests.
type providerMessage = wfprovider.Message

// compressMessages is a local alias for Compress used in QA tests.
func compressMessages(ctx context.Context, messages []wfprovider.Message, preserveCount int, prov wfprovider.Provider) ([]wfprovider.Message, string, error) {
	return Compress(ctx, messages, preserveCount, prov)
}

// loadBuiltinAgentDefs is a local alias for agent.LoadBuiltins used in QA tests.
func loadBuiltinAgentDefs() ([]agent.AgentDefinition, error) {
	return agent.LoadBuiltins()
}

// Ensure fmt is used (referenced in qa_test.go which imports it indirectly via this file).
var _ = fmt.Sprintf
