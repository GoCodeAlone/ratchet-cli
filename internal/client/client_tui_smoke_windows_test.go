//go:build tui_smoke && windows

package client

import (
	"context"
	"strings"
	"testing"
)

func TestConnectSmokeLoopbackRejectsNonLoopbackTargets(t *testing.T) {
	_, err := ConnectSmokeLoopback(context.Background(), "0.0.0.0:1234")
	if err == nil {
		t.Fatal("expected non-loopback target to fail")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("error %q does not name loopback requirement", err)
	}
}
