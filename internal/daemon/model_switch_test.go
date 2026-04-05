package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestUpdateProviderModel_UpdatesDB(t *testing.T) {
	engine := newTestEngine(t)
	svc := &Service{
		engine:      engine,
		broadcaster: NewSessionBroadcaster(),
	}

	// The mock provider "default" is inserted by newTestEngine with model="".
	_, err := svc.UpdateProviderModel(context.Background(), &pb.UpdateProviderModelReq{
		Alias: "default",
		Model: "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("UpdateProviderModel: %v", err)
	}

	// Verify the model was actually updated in the DB.
	var model string
	row := engine.DB.QueryRowContext(context.Background(), "SELECT model FROM llm_providers WHERE alias = ?", "default")
	if err := row.Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "claude-3-5-sonnet" {
		t.Errorf("expected model 'claude-3-5-sonnet', got %q", model)
	}
}

func TestUpdateProviderModel_InvalidatesCache(t *testing.T) {
	engine := newTestEngine(t)
	svc := &Service{
		engine:      engine,
		broadcaster: NewSessionBroadcaster(),
	}

	// Should not return error even on cache invalidation.
	_, err := svc.UpdateProviderModel(context.Background(), &pb.UpdateProviderModelReq{
		Alias: "default",
		Model: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("UpdateProviderModel: %v", err)
	}
}
