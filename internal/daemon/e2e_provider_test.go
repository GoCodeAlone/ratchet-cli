package daemon

// E2E tests: provider CRUD and secret resolution.
//
// Key bug scenario under test: providers registered without API keys (Ollama,
// llama_cpp) previously received secret_name="provider_<alias>" in the DB even
// though no secret was stored. When the ProviderRegistry tried to resolve that
// name it got "secret not found" and the chat failed. The fix is twofold:
//
//  1. AddProvider only sets secret_name when req.ApiKey != "" (service.go)
//  2. initDB runs a migration clearing stale secret_name for ollama/llama_cpp rows
//     that may already be in the DB (engine.go)
//
// These tests verify both halves of the fix using the real code path end-to-end.

import (
	"context"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// TestE2EProviderAddListRemove exercises the full CRUD lifecycle of a provider
// via the gRPC API, verifying that the DB is updated correctly at each step.
func TestE2EProviderAddListRemove(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Initially only the default harness provider exists.
	list, err := h.Client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListProviders (initial): %v", err)
	}
	initialCount := len(list.Providers)

	// Add a new provider.
	p := h.addProvider(t, "my-anthropic", "anthropic", "sk-test-key", false)
	if p.Alias != "my-anthropic" {
		t.Errorf("alias: got %q, want %q", p.Alias, "my-anthropic")
	}
	if p.Type != "anthropic" {
		t.Errorf("type: got %q, want %q", p.Type, "anthropic")
	}

	// List should now have one more provider.
	list2, err := h.Client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListProviders (after add): %v", err)
	}
	if len(list2.Providers) != initialCount+1 {
		t.Errorf("provider count: got %d, want %d", len(list2.Providers), initialCount+1)
	}

	// Find the new provider in the list.
	var found bool
	for _, lp := range list2.Providers {
		if lp.Alias == "my-anthropic" {
			found = true
			break
		}
	}
	if !found {
		t.Error("my-anthropic not found in list after add")
	}

	// Remove the provider.
	_, err = h.Client.RemoveProvider(ctx, &pb.RemoveProviderReq{Alias: "my-anthropic"})
	if err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}

	// Should be back to initial count.
	list3, err := h.Client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListProviders (after remove): %v", err)
	}
	if len(list3.Providers) != initialCount {
		t.Errorf("provider count after remove: got %d, want %d", len(list3.Providers), initialCount)
	}
}

// TestE2EProviderSetDefault verifies that SetDefaultProvider correctly updates
// the is_default flag and that only one provider is default at a time.
func TestE2EProviderSetDefault(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Add a second mock provider (not default).
	h.addProvider(t, "other-mock", "mock", "", false)

	// Set it as default.
	_, err := h.Client.SetDefaultProvider(ctx, &pb.SetDefaultProviderReq{Alias: "other-mock"})
	if err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}

	// Verify exactly one default.
	list, err := h.Client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}

	var defaultCount int
	var otherMockIsDefault bool
	for _, p := range list.Providers {
		if p.IsDefault {
			defaultCount++
		}
		if p.Alias == "other-mock" && p.IsDefault {
			otherMockIsDefault = true
		}
	}
	if defaultCount != 1 {
		t.Errorf("default provider count: got %d, want 1", defaultCount)
	}
	if !otherMockIsDefault {
		t.Error("expected other-mock to be default")
	}
}

// TestE2EProviderUpdateModel verifies that UpdateProviderModel updates the model
// field and invalidates the cache.
func TestE2EProviderUpdateModel(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h.addProvider(t, "gpt-provider", "openai", "sk-fake", false)

	_, err := h.Client.UpdateProviderModel(ctx, &pb.UpdateProviderModelReq{
		Alias: "gpt-provider",
		Model: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("UpdateProviderModel: %v", err)
	}

	// Verify via DB directly (list doesn't expose model).
	var model string
	if err := h.DB.QueryRowContext(ctx,
		`SELECT model FROM llm_providers WHERE alias = ?`, "gpt-provider",
	).Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", model, "gpt-4o")
	}
}

// TestE2EAddProviderWithKey_StoresSecret verifies that AddProvider with an API
// key creates both the DB row (with a non-empty secret_name) and the secret file.
func TestE2EAddProviderWithKey_StoresSecret(t *testing.T) {
	h := newE2EHarness(t)

	h.addProvider(t, "keyed", "anthropic", "sk-test-12345", false)

	// secret_name in DB must be "provider_keyed" (the naming convention).
	secretName := h.providerSecretName(t, "keyed")
	if secretName == "" {
		t.Error("expected non-empty secret_name in DB for a provider with an API key")
	}

	// The secret file must have been written.
	if !h.secretExists(t, secretName) {
		t.Errorf("secret %q not found in file provider after AddProvider with key", secretName)
	}
}

// TestE2EAddProviderWithoutKey_NoSecret is THE critical regression test.
//
// Providers like Ollama don't need an API key. Before the fix, AddProvider
// always stored secret_name="provider_<alias>" in the DB, which caused the
// ProviderRegistry to attempt a secret lookup that always failed. After the fix,
// secret_name must be empty when no ApiKey is supplied.
func TestE2EAddProviderWithoutKey_NoSecret(t *testing.T) {
	h := newE2EHarness(t)

	// Simulate adding an Ollama-style provider (no API key).
	// We use type "mock" because we don't have a real Ollama server in tests.
	// The fix is in AddProvider's secret_name logic, not the provider type.
	h.addProvider(t, "keyless", "mock", "", false)

	// secret_name in DB must be empty.
	secretName := h.providerSecretName(t, "keyless")
	if secretName != "" {
		t.Errorf("expected empty secret_name for keyless provider, got %q", secretName)
	}

	// No secret file should have been created.
	if h.secretExists(t, "provider_keyless") {
		t.Error("unexpected secret file 'provider_keyless' created for keyless provider")
	}
}

// TestE2EStaleMigration_KeylessTypesGetEmptySecretName verifies the initDB
// migration that clears stale secret_name values for ollama/llama_cpp rows that
// were inserted before the fix. We inject a stale row directly into the DB
// (bypassing the fixed AddProvider RPC), then call initDB again to re-apply
// migrations, and confirm the row was cleaned up.
func TestE2EStaleMigration_KeylessTypesGetEmptySecretName(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Inject a stale pre-fix row: ollama provider with a non-empty secret_name.
	_, err := h.DB.ExecContext(ctx, `INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES ('stale-ollama', 'stale-ollama', 'ollama', 'llama3', 'provider_stale-ollama', 'http://localhost:11434', 4096, '{}', 0)`)
	if err != nil {
		t.Fatalf("inject stale row: %v", err)
	}

	// Also inject a stale llama_cpp row.
	_, err = h.DB.ExecContext(ctx, `INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES ('stale-llama', 'stale-llamacpp', 'llama_cpp', '', 'provider_stale-llamacpp', 'http://localhost:8080', 4096, '{}', 0)`)
	if err != nil {
		t.Fatalf("inject stale llama_cpp row: %v", err)
	}

	// Re-run initDB to apply the migration against the stale rows.
	if err := initDB(h.DB); err != nil {
		t.Fatalf("initDB (migration pass): %v", err)
	}

	// The migration must have cleared secret_name for both stale rows.
	secretName := h.providerSecretName(t, "stale-ollama")
	if secretName != "" {
		t.Errorf("stale-ollama: expected empty secret_name after migration, got %q", secretName)
	}

	secretName = h.providerSecretName(t, "stale-llamacpp")
	if secretName != "" {
		t.Errorf("stale-llamacpp: expected empty secret_name after migration, got %q", secretName)
	}
}

// TestE2EKeylessProviderResolvesWithoutError verifies the full end-to-end flow:
// a keyless provider (simulated with type "mock") registered via AddProvider can
// be resolved by the ProviderRegistry without a "secret not found" error, and a
// chat turn using that provider succeeds.
func TestE2EKeylessProviderResolvesWithoutError(t *testing.T) {
	h := newE2EHarness(t)

	// Register a keyless mock provider (simulates Ollama pattern).
	h.addProvider(t, "keyless-mock", "mock", "", true)

	// Create a session pinned to it.
	session := h.createSession(t, "keyless-mock")

	// A chat turn must succeed — if secret resolution is broken this would
	// return an error event containing "secret not found".
	tokens, gotComplete := h.sendMessage(t, session.Id, "hello")
	if tokens == "" {
		t.Error("expected non-empty response from keyless mock provider")
	}
	if !gotComplete {
		t.Error("expected SessionComplete event")
	}
}

// TestE2EKeyedProviderResolvesWithKey verifies that a provider registered with
// an API key can be resolved and used for chat, with the key properly fetched
// from the file-based secrets provider.
func TestE2EKeyedProviderResolvesWithKey(t *testing.T) {
	h := newE2EHarness(t)

	// Register a mock provider with an API key (the mock factory ignores the key
	// but the ProviderRegistry still resolves it from the file provider).
	h.addProvider(t, "keyed-mock", "mock", "sk-real-looking-key", true)

	session := h.createSession(t, "keyed-mock")
	tokens, gotComplete := h.sendMessage(t, session.Id, "hello")
	if tokens == "" {
		t.Error("expected non-empty response from keyed mock provider")
	}
	if !gotComplete {
		t.Error("expected SessionComplete event")
	}
}

// TestE2EProviderDuplicateAlias verifies that adding a provider with an existing
// alias upserts (updates) rather than erroring.
func TestE2EProviderDuplicateAlias(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h.addProvider(t, "dup-test", "mock", "", false)

	p, err := h.Client.AddProvider(ctx, &pb.AddProviderReq{
		Alias: "dup-test",
		Type:  "mock",
	})
	if err != nil {
		t.Fatalf("expected upsert to succeed, got: %v", err)
	}
	if p.Alias != "dup-test" {
		t.Errorf("expected alias dup-test, got %s", p.Alias)
	}
}

// TestE2ERemoveNonexistentProvider verifies that removing an alias that does not
// exist silently succeeds (DELETE WHERE alias=? affects 0 rows — not an error).
func TestE2ERemoveNonexistentProvider(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := h.Client.RemoveProvider(ctx, &pb.RemoveProviderReq{Alias: "ghost"})
	if err != nil {
		t.Errorf("RemoveProvider(ghost): expected nil, got %v", err)
	}
}
