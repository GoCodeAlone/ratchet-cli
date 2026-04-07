package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// chatEventToUpdates converts a ratchet ChatEvent into ACP SessionUpdate(s).
// Returns nil if the event type has no ACP equivalent.
func chatEventToUpdates(ev *pb.ChatEvent) []acpsdk.SessionUpdate {
	switch e := ev.Event.(type) {
	case *pb.ChatEvent_Token:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateAgentMessageText(e.Token.Content),
		}

	case *pb.ChatEvent_Thinking:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateAgentThoughtText(e.Thinking.Content),
		}

	case *pb.ChatEvent_ToolStart:
		return []acpsdk.SessionUpdate{
			acpsdk.StartToolCall(
				acpsdk.ToolCallId(e.ToolStart.CallId),
				e.ToolStart.ToolName,
				acpsdk.WithStartStatus(acpsdk.ToolCallStatusInProgress),
				acpsdk.WithStartRawInput(e.ToolStart.ArgumentsJson),
			),
		}

	case *pb.ChatEvent_ToolResult:
		st := acpsdk.ToolCallStatusCompleted
		if !e.ToolResult.Success {
			st = acpsdk.ToolCallStatusFailed
		}
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateToolCall(
				acpsdk.ToolCallId(e.ToolResult.CallId),
				acpsdk.WithUpdateStatus(st),
				acpsdk.WithUpdateRawOutput(e.ToolResult.ResultJson),
			),
		}

	case *pb.ChatEvent_Permission:
		// Permission requests are handled via ACP's RequestPermission RPC,
		// not session updates. Return nil here; the agent handles it separately.
		return nil

	case *pb.ChatEvent_PlanProposed:
		entries := make([]acpsdk.PlanEntry, len(e.PlanProposed.Steps))
		for i, step := range e.PlanProposed.Steps {
			entries[i] = acpsdk.PlanEntry{
				Content: step.Description,
				Status:  mapPlanStepStatus(step.Status),
			}
		}
		return []acpsdk.SessionUpdate{
			acpsdk.UpdatePlan(entries...),
		}

	case *pb.ChatEvent_PlanStepUpdate:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdatePlan(acpsdk.PlanEntry{
				Content: e.PlanStepUpdate.Description,
				Status:  mapPlanStepStatus(e.PlanStepUpdate.Status),
			}),
		}

	case *pb.ChatEvent_Error:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateAgentMessageText("[error] " + e.Error.Message),
		}

	case *pb.ChatEvent_Complete:
		// Completion is signaled by returning PromptResponse, not via update.
		return nil

	case *pb.ChatEvent_AgentSpawned:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateAgentMessageText("[agent spawned] " + e.AgentSpawned.AgentName + " (" + e.AgentSpawned.Role + ")"),
		}

	case *pb.ChatEvent_AgentMessage:
		return []acpsdk.SessionUpdate{
			acpsdk.UpdateAgentMessageText("[" + e.AgentMessage.FromAgent + "] " + e.AgentMessage.Content),
		}

	default:
		return nil
	}
}

func mapPlanStepStatus(s string) acpsdk.PlanEntryStatus {
	switch s {
	case "completed":
		return acpsdk.PlanEntryStatusCompleted
	case "in_progress":
		return acpsdk.PlanEntryStatusInProgress
	case "failed":
		return acpsdk.PlanEntryStatusCompleted // ACP has no "failed" status
	default:
		return acpsdk.PlanEntryStatusPending
	}
}
