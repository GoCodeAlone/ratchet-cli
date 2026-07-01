package daemon

import (
	"context"
	"strings"
	"testing"
)

func TestHarnessSmokeMockProviderSessionRoundTrip(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	health, err := h.Client.Health(ctx, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !health.Healthy {
		t.Fatal("expected healthy harness daemon")
	}

	session := h.createSession(t, "e2e-mock")
	text, complete := h.sendMessage(t, session.Id, "harness smoke")
	if !complete {
		t.Fatal("expected complete event")
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("expected mock provider token output")
	}
}
