package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
)

type fixtureAgent struct {
	conn        *acpsdk.AgentSideConnection
	echoSession bool
	loadSession bool
}

var _ acpsdk.Agent = (*fixtureAgent)(nil)
var _ acpsdk.AgentLoader = (*fixtureAgent)(nil)

func (*fixtureAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (a *fixtureAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		AgentInfo:         &acpsdk.Implementation{Name: "ratchet-fixture-agent", Version: "test"},
		AgentCapabilities: acpsdk.AgentCapabilities{LoadSession: a.loadSession},
	}, nil
}

func (*fixtureAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (*fixtureAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "fixture-session"}, nil
}

func (*fixtureAgent) LoadSession(context.Context, acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	return acpsdk.LoadSessionResponse{}, nil
}

func (a *fixtureAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	var prompt strings.Builder
	for _, block := range params.Prompt {
		if block.Text != nil {
			prompt.WriteString(block.Text.Text)
		}
	}
	text := "fixture: " + prompt.String()
	if a.echoSession {
		text = fmt.Sprintf("fixture: %s: %s", params.SessionId, prompt.String())
	}
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: params.SessionId,
		Update:    acpsdk.UpdateAgentMessageText(text),
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*fixtureAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func main() {
	echoSession := flag.Bool("echo-session", false, "include ACP session id in prompt responses")
	loadSession := flag.Bool("load-session", false, "advertise and accept ACP session/load")
	flag.Parse()

	agent := &fixtureAgent{echoSession: *echoSession, loadSession: *loadSession}
	conn := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.conn = conn
	<-conn.Done()
}
