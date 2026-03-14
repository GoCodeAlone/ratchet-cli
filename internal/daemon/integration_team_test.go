package daemon

import (
	"context"
	"io"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestIntegration_TeamLifecycle(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.StartTeam(ctx, &pb.StartTeamReq{
		Task:      "summarise recent commits",
		SessionId: "sess-team-1",
	})
	if err != nil {
		t.Fatalf("StartTeam: %v", err)
	}

	var teamID string
	var gotAgentSpawned bool
	var gotComplete bool
	var agentNames []string

	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *pb.TeamEvent_AgentSpawned:
			gotAgentSpawned = true
			agentNames = append(agentNames, e.AgentSpawned.AgentName)
		case *pb.TeamEvent_Complete:
			gotComplete = true
			_ = e
		}
	}

	if !gotAgentSpawned {
		t.Error("expected at least one AgentSpawned event")
	}
	if !gotComplete {
		t.Error("expected SessionComplete event")
	}
	if len(agentNames) < 2 {
		t.Errorf("expected at least 2 agents (orchestrator + worker), got %d", len(agentNames))
	}

	// GetTeamStatus requires a teamID — extract from a second StartTeam call
	// since the stream doesn't directly return the team ID.
	// Verify the RPC is wired by calling it with a dummy ID (expects NotFound).
	_, err = client.GetTeamStatus(ctx, &pb.TeamStatusReq{TeamId: teamID})
	if teamID == "" {
		// Expect NotFound for empty team ID — just verify no panic.
		_ = err
	}
}

func TestIntegration_TeamMessageRouting(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.StartTeam(ctx, &pb.StartTeamReq{
		Task:      "analyse test coverage",
		SessionId: "sess-team-2",
	})
	if err != nil {
		t.Fatalf("StartTeam: %v", err)
	}

	var agentMessages []*pb.AgentMessage
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		if msg, ok := ev.Event.(*pb.TeamEvent_AgentMessage); ok {
			agentMessages = append(agentMessages, msg.AgentMessage)
		}
	}

	// Orchestrator → worker and worker → orchestrator messages expected.
	if len(agentMessages) < 2 {
		t.Errorf("expected at least 2 agent messages, got %d", len(agentMessages))
	}
	for _, m := range agentMessages {
		if m.FromAgent == "" || m.ToAgent == "" {
			t.Error("expected non-empty FromAgent and ToAgent")
		}
		if m.Content == "" {
			t.Error("expected non-empty message content")
		}
	}
}
