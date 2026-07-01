package main

import (
	"context"
	"os"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
)

type fixtureAgent struct {
	conn *acpsdk.AgentSideConnection
}

var _ acpsdk.Agent = (*fixtureAgent)(nil)

func (*fixtureAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*fixtureAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		AgentInfo: &acpsdk.Implementation{Name: "ratchet-fixture-agent", Version: "test"},
	}, nil
}

func (*fixtureAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (*fixtureAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "fixture-session"}, nil
}

func (a *fixtureAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	var prompt strings.Builder
	for _, block := range params.Prompt {
		if block.Text != nil {
			prompt.WriteString(block.Text.Text)
		}
	}
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: params.SessionId,
		Update:    acpsdk.UpdateAgentMessageText("fixture: " + prompt.String()),
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*fixtureAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func main() {
	agent := &fixtureAgent{}
	conn := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.conn = conn
	<-conn.Done()
}
