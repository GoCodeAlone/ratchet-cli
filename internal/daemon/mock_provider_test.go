package daemon

import (
	"context"
	"database/sql"
	"testing"

	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
)

// mockProvider is a scripted provider that returns fixed text responses.
type mockProvider struct {
	responses []string
	idx       int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) AuthModeInfo() provider.AuthModeInfo {
	return provider.AuthModeInfo{Mode: "test", ServerSafe: true}
}

func (m *mockProvider) Chat(_ context.Context, _ []provider.Message, _ []provider.ToolDef) (*provider.Response, error) {
	resp := m.next()
	return &provider.Response{Content: resp}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ []provider.Message, _ []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 2)
	text := m.next()
	ch <- provider.StreamEvent{Type: "text", Text: text}
	ch <- provider.StreamEvent{Type: "done"}
	close(ch)
	return ch, nil
}

func (m *mockProvider) next() string {
	if len(m.responses) == 0 {
		return "task completed"
	}
	r := m.responses[m.idx%len(m.responses)]
	m.idx++
	return r
}

// memSecretsProvider is an in-memory secrets provider for tests.
type memSecretsProvider struct{}

func (m *memSecretsProvider) Name() string                                    { return "mem" }
func (m *memSecretsProvider) Get(_ context.Context, _ string) (string, error) { return "", nil }
func (m *memSecretsProvider) Set(_ context.Context, _, _ string) error        { return nil }
func (m *memSecretsProvider) Delete(_ context.Context, _ string) error        { return nil }
func (m *memSecretsProvider) List(_ context.Context) ([]string, error)        { return nil, nil }

var _ secrets.Provider = (*memSecretsProvider)(nil)

// newTestEngine creates an EngineContext backed by an in-memory SQLite DB with a
// built-in mock provider registered as default. Safe to call from any test.
func newTestEngine(t *testing.T) *EngineContext {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := initDB(db); err != nil {
		t.Fatalf("initDB: %v", err)
	}

	// Add the llm_providers table (initDB doesn't create it in the test path)
	_, err = db.Exec(`INSERT INTO llm_providers (id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES ('mock1', 'default', 'mock', '', '', '', 4096, '{}', 1)`)
	if err != nil {
		t.Fatalf("insert mock provider: %v", err)
	}

	sp := &memSecretsProvider{}
	reg := ratchetplugin.NewProviderRegistry(db, sp)

	return &EngineContext{
		DB:               db,
		ProviderRegistry: reg,
		ToolRegistry:     ratchetplugin.NewToolRegistry(),
	}
}
