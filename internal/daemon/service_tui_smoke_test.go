//go:build tui_smoke && !windows

package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestSmokeServiceInitializesSafeJobProviders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")

	session, cleanup, err := StartTUISmokeDaemon(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("StartTUISmokeDaemon: %v", err)
	}
	defer cleanup()
	if session == nil {
		t.Fatal("expected initial smoke session")
	}
	if session.GetWorkingDir() != root {
		t.Fatalf("expected smoke session working dir %q, got %q", root, session.GetWorkingDir())
	}

	svc := newTUISmokeServiceForTest(t, root)
	if svc.engine == nil {
		t.Fatal("expected smoke engine")
	}
	if svc.engine.MCPDiscoverer != nil {
		t.Fatal("smoke service must disable MCP discovery")
	}
	if len(svc.engine.PluginSkills) != 0 || len(svc.engine.PluginAgents) != 0 ||
		len(svc.engine.PluginCommands) != 0 || len(svc.engine.PluginDaemons) != 0 {
		t.Fatal("smoke service must not load plugin capabilities")
	}
	if svc.autorespond != nil {
		t.Fatal("smoke service must not load autoresponder config from host workdir")
	}
	if svc.cron != nil {
		t.Fatal("smoke service must not start cron/background scheduler")
	}
	if svc.jobs == nil {
		t.Fatal("expected smoke job registry")
	}
}

func TestSmokeServiceListJobsHasMarkerOrEmptyState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")
	_, cleanup, err := StartTUISmokeDaemon(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("StartTUISmokeDaemon: %v", err)
	}
	defer cleanup()

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("connect smoke socket: %v", err)
	}
	defer conn.Close()

	client := pb.NewRatchetDaemonClient(conn)
	list, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if list == nil {
		t.Fatal("expected job list")
	}
	for _, job := range list.Jobs {
		if job.GetType() == "" || job.GetName() == "" {
			t.Fatalf("smoke job should expose test-observable type/name: %#v", job)
		}
	}
}
