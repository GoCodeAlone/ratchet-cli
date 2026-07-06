package providerauth

import (
	"context"
	"testing"
)

func TestListModelsDelegatesToProviderPackage(t *testing.T) {
	models, err := ListModels(context.Background(), "mock", "", "")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "mock-default" {
		t.Fatalf("model ID = %q", models[0].ID)
	}
}

func TestListModelsUnsupported(t *testing.T) {
	_, err := ListModels(context.Background(), "unknown", "", "")
	if err == nil {
		t.Fatal("expected error for unsupported provider type")
	}
}
