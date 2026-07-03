//go:build tui_smoke && !windows

package daemon

import (
	"context"
	"fmt"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// StartTUISmokeDaemon starts the private daemon used by ratchet-tui-smoke.
func StartTUISmokeDaemon(_ context.Context, _, _ string) (*pb.Session, func(), error) {
	return nil, func() {}, fmt.Errorf("tui smoke daemon service is not wired yet")
}
