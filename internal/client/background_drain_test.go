package client

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestBackgroundDrainClientWrappers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := daemon.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	manager := &clientBackgroundDrainManager{now: now}
	service, err := daemon.NewService(t.Context(), daemon.WithACPBackgroundDrainManager(manager))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(service.Close)
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, service)
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	go func() { _ = server.Serve(listener) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &Client{conn: conn, daemon: pb.NewRatchetDaemonClient(conn)}

	started, err := client.StartACPBackgroundDrain(t.Context(), "session-1", "codex", true)
	if err != nil || started.GetState() != "running" {
		t.Fatalf("start = %#v, %v", started, err)
	}
	stopped, err := client.StopACPBackgroundDrain(t.Context(), "session-1")
	if err != nil || stopped.GetState() != "disabled" {
		t.Fatalf("stop = %#v, %v", stopped, err)
	}
	got, err := client.GetACPBackgroundDrain(t.Context(), "session-1")
	if err != nil || got.GetSessionId() != "session-1" {
		t.Fatalf("get = %#v, %v", got, err)
	}
	listed, err := client.ListACPBackgroundDrains(t.Context())
	if err != nil || len(listed.GetDrains()) != 1 {
		t.Fatalf("list = %#v, %v", listed, err)
	}
	if manager.sessionID != "session-1" || manager.profile != "codex" || !manager.acknowledged {
		t.Fatalf("manager args = %q/%q/%t", manager.sessionID, manager.profile, manager.acknowledged)
	}
}

type clientBackgroundDrainManager struct {
	now          time.Time
	sessionID    string
	profile      string
	acknowledged bool
}

func (m *clientBackgroundDrainManager) Start(sessionID, profile string, acknowledged bool) (acpclient.BackgroundStatus, error) {
	m.sessionID = sessionID
	m.profile = profile
	m.acknowledged = acknowledged
	return m.status("running", "started"), nil
}

func (m *clientBackgroundDrainManager) Stop(sessionID string) (acpclient.BackgroundStatus, error) {
	m.sessionID = sessionID
	return m.status("disabled", "stopped"), nil
}

func (m *clientBackgroundDrainManager) Get(sessionID string) (acpclient.BackgroundStatus, error) {
	m.sessionID = sessionID
	return m.status("running", "started"), nil
}

func (m *clientBackgroundDrainManager) List() ([]acpclient.BackgroundStatus, error) {
	return []acpclient.BackgroundStatus{m.status("running", "started")}, nil
}

func (*clientBackgroundDrainManager) Shutdown() {}

func (m *clientBackgroundDrainManager) status(state, outcome string) acpclient.BackgroundStatus {
	if m.sessionID == "" {
		m.sessionID = "session-1"
	}
	if m.profile == "" {
		m.profile = "codex"
	}
	return acpclient.BackgroundStatus{
		SessionID: m.sessionID, Profile: m.profile, DescriptorHash: "pinned-hash",
		State: state, Outcome: outcome, AcknowledgedAt: m.now, StartedAt: m.now, UpdatedAt: m.now,
	}
}

var _ daemon.ACPBackgroundDrainManager = (*clientBackgroundDrainManager)(nil)
