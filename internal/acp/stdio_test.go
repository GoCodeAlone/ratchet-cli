package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	acpsdk "github.com/coder/acp-go-sdk"
)

type stdioSmokeClient struct {
	updates chan acpsdk.SessionNotification
}

var _ acpsdk.Client = (*stdioSmokeClient)(nil)

func (c *stdioSmokeClient) SessionUpdate(_ context.Context, n acpsdk.SessionNotification) error {
	select {
	case c.updates <- n:
	default:
	}
	return nil
}

func (*stdioSmokeClient) RequestPermission(_ context.Context, p acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	if len(p.Options) == 0 {
		return acpsdk.RequestPermissionResponse{
			Outcome: acpsdk.RequestPermissionOutcome{
				Cancelled: &acpsdk.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}
	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.RequestPermissionOutcome{
			Selected: &acpsdk.RequestPermissionOutcomeSelected{OptionId: p.Options[0].OptionId},
		},
	}, nil
}

func (*stdioSmokeClient) ReadTextFile(_ context.Context, p acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	if !filepath.IsAbs(p.Path) {
		return acpsdk.ReadTextFileResponse{}, fmt.Errorf("path must be absolute: %s", p.Path)
	}
	b, err := os.ReadFile(p.Path)
	if err != nil {
		return acpsdk.ReadTextFileResponse{}, err
	}
	return acpsdk.ReadTextFileResponse{Content: string(b)}, nil
}

func (*stdioSmokeClient) WriteTextFile(_ context.Context, p acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	if !filepath.IsAbs(p.Path) {
		return acpsdk.WriteTextFileResponse{}, fmt.Errorf("path must be absolute: %s", p.Path)
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return acpsdk.WriteTextFileResponse{}, err
	}
	return acpsdk.WriteTextFileResponse{}, os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

func (*stdioSmokeClient) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{TerminalId: "stdio-smoke-terminal"}, nil
}

func (*stdioSmokeClient) KillTerminalCommand(context.Context, acpsdk.KillTerminalCommandRequest) (acpsdk.KillTerminalCommandResponse, error) {
	return acpsdk.KillTerminalCommandResponse{}, nil
}

func (*stdioSmokeClient) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{Output: "ok"}, nil
}

func (*stdioSmokeClient) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, nil
}

func (*stdioSmokeClient) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, nil
}

func TestACPStdioPromptSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	if err := daemon.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	svc, err := daemon.NewService(ctx)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _, _ = svc.Shutdown(context.Background(), nil) })

	if _, err := svc.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     "stdio-mock",
		Type:      "mock",
		IsDefault: true,
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentR.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientR.Close()
		_ = agentToClientW.Close()
	})

	agent := NewRatchetAgent(svc)
	agentConn := acpsdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.SetConnection(agentConn)
	client := &stdioSmokeClient{updates: make(chan acpsdk.SessionNotification, 16)}
	clientConn := acpsdk.NewClientSideConnection(client, clientToAgentW, agentToClientR)

	initResp, err := clientConn.Initialize(ctx, acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpsdk.ClientCapabilities{
			Fs: acpsdk.FileSystemCapability{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initResp.AgentInfo == nil || initResp.AgentInfo.Name != "ratchet" {
		t.Fatalf("agent info = %#v, want ratchet", initResp.AgentInfo)
	}

	session, err := clientConn.NewSession(ctx, acpsdk.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if session.SessionId == "" {
		t.Fatal("expected ACP session id")
	}

	if _, err := clientConn.Prompt(ctx, acpsdk.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("stdio smoke")},
	}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	var received strings.Builder
	for {
		select {
		case n := <-client.updates:
			if n.SessionId != session.SessionId {
				t.Fatalf("SessionUpdate session id = %q, want %q", n.SessionId, session.SessionId)
			}
			if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
				received.WriteString(n.Update.AgentMessageChunk.Content.Text.Text)
			}
		default:
			if strings.TrimSpace(received.String()) == "" {
				t.Fatal("expected at least one agent message update over ACP stdio")
			}
			return
		}
	}
}
