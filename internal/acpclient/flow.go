package acpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoCodeAlone/acpx-go/flowjson"
)

var ErrInvalidFlowDefinition = flowjson.ErrInvalidDefinition

const (
	FlowNodeTypeACP        = flowjson.NodeTypeACP
	FlowNodeTypeAction     = flowjson.NodeTypeAction
	FlowNodeTypeCompute    = flowjson.NodeTypeCompute
	FlowNodeTypeCheckpoint = flowjson.NodeTypeCheckpoint

	FlowRunStatusRunning   = "running"
	FlowRunStatusCompleted = "completed"
	FlowRunStatusFailed    = "failed"

	FlowStepStatusCompleted = "completed"
	FlowStepStatusFailed    = "failed"
)

type FlowDefinition = flowjson.Definition
type FlowNode = flowjson.Node
type FlowEdge = flowjson.Edge
type FlowSwitchEdge = flowjson.SwitchEdge

type FlowRunState struct {
	RunID  string           `json:"run_id"`
	Status string           `json:"status"`
	Steps  []FlowStepRecord `json:"steps"`
}

type FlowStepRecord struct {
	NodeID string          `json:"node_id"`
	Type   string          `json:"type,omitempty"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func LoadFlowDefinition(path string) (FlowDefinition, error) {
	var def FlowDefinition
	b, err := os.ReadFile(path)
	if err != nil {
		return FlowDefinition{}, err
	}
	if err := json.Unmarshal(b, &def); err != nil {
		return FlowDefinition{}, fmt.Errorf("%w: %v", ErrInvalidFlowDefinition, err)
	}
	if err := def.Validate(); err != nil {
		return FlowDefinition{}, err
	}
	return def, nil
}

func safeFlowIdentifier(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	return id == filepath.Base(id) && !strings.ContainsAny(id, `/\`)
}
