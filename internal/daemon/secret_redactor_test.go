package daemon

import (
	"context"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestEngineSecretRedactorRedactsToolCallArguments(t *testing.T) {
	base := secrets.NewRedactor()
	base.AddValue("api-key", "sk-tool-secret")

	redactor := newEngineSecretRedactor(base)
	msg := &provider.Message{
		ToolCalls: []provider.ToolCall{
			{
				ID:   "call-1",
				Name: "example",
				Arguments: map[string]any{
					"token": "sk-tool-secret",
					"nested": map[string]any{
						"list": []any{"prefix sk-tool-secret suffix"},
					},
				},
			},
		},
	}

	redactor.CheckAndRedact(msg)

	args := msg.ToolCalls[0].Arguments
	if got := args["token"]; got != "[REDACTED:api-key]" {
		t.Fatalf("token argument = %#v", got)
	}
	nested := args["nested"].(map[string]any)
	list := nested["list"].([]any)
	if got := list[0]; got != "prefix [REDACTED:api-key] suffix" {
		t.Fatalf("nested list argument = %#v", got)
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
	var secretName string
	if err := h.DB.QueryRowContext(ctx,
		`SELECT secret_name FROM llm_providers WHERE alias = 'redact-test'`,
	).Scan(&secretName); err != nil {
		t.Fatalf("query secret name: %v", err)
	}
	if !strings.HasPrefix(secretName, "provider-v2-") {
		t.Fatalf("secret name = %q, want versioned provider key", secretName)
	}

	got := h.Svc.engine.SecretRedactor.Redact("provider leaked sk-new-provider-secret")
	want := "provider leaked [REDACTED:" + secretName + "]"
	if got != want {
		t.Fatalf("SecretRedactor.Redact() = %q, want %q", got, want)
	}
}

func TestAddProviderRejectsPathLikeAliasForSecrets(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	_, err := h.Client.AddProvider(ctx, &pb.AddProviderReq{
		Alias:  "../escape",
		Type:   "mock",
		Model:  "scripted",
		ApiKey: "sk-path-secret",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AddProvider() code = %v, want %v (err=%v)", status.Code(err), codes.InvalidArgument, err)
	}
}
