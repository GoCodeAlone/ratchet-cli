package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
)

func TestEngineSecretRedactorRedactsProviderMessages(t *testing.T) {
	base := secrets.NewRedactor()
	base.AddValue("api-key", "sk-test-secret")

	redactor := newEngineSecretRedactor(base)
	var _ executor.SecretRedactor = redactor

	if got := redactor.Redact("token sk-test-secret"); got != "token [REDACTED:api-key]" {
		t.Fatalf("Redact() = %q", got)
	}

	msg := &provider.Message{Content: "message with sk-test-secret"}
	redactor.CheckAndRedact(msg)

	if got := msg.Content; got != "message with [REDACTED:api-key]" {
		t.Fatalf("CheckAndRedact() content = %q", got)
	}
}

func TestAddProviderArmsSecretRedactor(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	_, err := h.Client.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     "redact-test",
		Type:      "mock",
		Model:     "scripted",
		ApiKey:    "sk-new-provider-secret",
		IsDefault: false,
	})
	if err != nil {
		t.Fatalf("AddProvider() error = %v", err)
	}

	got := h.Svc.engine.SecretRedactor.Redact("provider leaked sk-new-provider-secret")
	want := "provider leaked [REDACTED:provider_redact-test]"
	if got != want {
		t.Fatalf("SecretRedactor.Redact() = %q, want %q", got, want)
	}
}
