package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestACPBackgroundDrainRPCsCrossRealGRPCBoundary(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	manager := &fakeACPBackgroundDrainManager{
		start: func(sessionID, profile string, acknowledged bool) (acpclient.BackgroundStatus, error) {
			if sessionID != "session-1" || profile != "codex" || !acknowledged {
				t.Fatalf("start args = %q/%q/%t", sessionID, profile, acknowledged)
			}
			return backgroundDrainTestStatus("session-1", "codex", acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeStarted, now), nil
		},
		stop: func(sessionID string) (acpclient.BackgroundStatus, error) {
			return backgroundDrainTestStatus(sessionID, "codex", acpclient.BackgroundStateDisabled, acpclient.BackgroundOutcomeStopped, now), nil
		},
		get: func(sessionID string) (acpclient.BackgroundStatus, error) {
			return backgroundDrainTestStatus(sessionID, "codex", acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeStarted, now), nil
		},
		list: func() ([]acpclient.BackgroundStatus, error) {
			return []acpclient.BackgroundStatus{
				backgroundDrainTestStatus("session-1", "codex", acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeStarted, now),
				backgroundDrainTestStatus("session-2", "claude", acpclient.BackgroundStateDisabled, acpclient.BackgroundOutcomeStopped, now),
			}, nil
		},
	}
	addr := startTestGRPCServer(t, &Service{acpBackground: manager})
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewRatchetDaemonClient(conn)

	started, err := client.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
		SessionId:    "session-1",
		Profile:      "codex",
		Acknowledged: true,
	})
	if err != nil {
		t.Fatalf("StartACPBackgroundDrain: %v", err)
	}
	assertBackgroundDrainProto(t, started, "session-1", "codex", acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeStarted, now)

	stopped, err := client.StopACPBackgroundDrain(t.Context(), &pb.ACPBackgroundDrainReq{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("StopACPBackgroundDrain: %v", err)
	}
	assertBackgroundDrainProto(t, stopped, "session-1", "codex", acpclient.BackgroundStateDisabled, acpclient.BackgroundOutcomeStopped, now)

	got, err := client.GetACPBackgroundDrain(t.Context(), &pb.ACPBackgroundDrainReq{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("GetACPBackgroundDrain: %v", err)
	}
	assertBackgroundDrainProto(t, got, "session-1", "codex", acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeStarted, now)

	listed, err := client.ListACPBackgroundDrains(t.Context(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListACPBackgroundDrains: %v", err)
	}
	if len(listed.GetDrains()) != 2 || listed.GetDrains()[1].GetSessionId() != "session-2" {
		t.Fatalf("listed drains = %#v", listed.GetDrains())
	}
}

func TestACPBackgroundDrainRPCMapsStableErrorCodes(t *testing.T) {
	for _, test := range []struct {
		name string
		req  *pb.StartACPBackgroundDrainReq
	}{
		{name: "nil_request"},
		{name: "session", req: &pb.StartACPBackgroundDrainReq{Profile: "codex", Acknowledged: true}},
		{name: "profile", req: &pb.StartACPBackgroundDrainReq{SessionId: "session-1", Acknowledged: true}},
		{name: "acknowledgement", req: &pb.StartACPBackgroundDrainReq{SessionId: "session-1", Profile: "codex"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeACPBackgroundDrainManager{start: func(string, string, bool) (acpclient.BackgroundStatus, error) {
				t.Fatal("manager called with invalid request")
				return acpclient.BackgroundStatus{}, nil
			}}
			svc := &Service{acpBackground: manager}
			_, err := svc.StartACPBackgroundDrain(t.Context(), test.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %s, want %s: %v", status.Code(err), codes.InvalidArgument, err)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "session", err: acpclient.ErrSessionNotFound, code: codes.NotFound},
		{name: "profile", err: acpclient.ErrProfileNotFound, code: codes.NotFound},
		{name: "untrusted", err: acpclient.ErrBackgroundProfileUntrusted, code: codes.FailedPrecondition},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &Service{acpBackground: &fakeACPBackgroundDrainManager{
				start: func(string, string, bool) (acpclient.BackgroundStatus, error) {
					return acpclient.BackgroundStatus{}, test.err
				},
			}}
			_, err := svc.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
				SessionId: "session-1", Profile: "fixture", Acknowledged: true,
			})
			if status.Code(err) != test.code {
				t.Fatalf("code = %s, want %s: %v", status.Code(err), test.code, err)
			}
		})
	}
}

func TestACPBackgroundDrainUntrustedProfileLaunchesNoWatcher(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 14, 9, 5, 0, 0, time.UTC)
	sessions := acpclient.NewStore(filepath.Join(dir, "sessions.json"))
	if err := sessions.Upsert(acpclient.SessionRecord{ID: "session-1", Status: acpclient.SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	var watchers atomic.Int32
	manager := acpclient.NewBackgroundManager(
		sessions,
		acpclient.NewBackgroundStore(filepath.Join(dir, "background.json")),
		acpclient.NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		acpclient.BackgroundManagerOptions{
			Context: t.Context(),
			Now:     func() time.Time { return now },
			Resolver: func(string) (acpclient.ResolvedBackgroundProfile, error) {
				return acpclient.ResolvedBackgroundProfile{
					Spec:           acpclient.AgentSpec{Name: "fixture", Command: "fixture"},
					DescriptorHash: "stale-hash",
					TrustValid:     false,
				}, nil
			},
			Watcher: func(context.Context, *acpclient.Store, acpclient.AgentSpec, acpclient.RunOptions, string, acpclient.WatchOptions, func(acpclient.WatchCycle)) (acpclient.WatchResult, error) {
				watchers.Add(1)
				return acpclient.WatchResult{}, nil
			},
		},
	)
	t.Cleanup(manager.Shutdown)
	svc := &Service{acpBackground: manager}

	_, err := svc.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
		SessionId: "session-1", Profile: "fixture", Acknowledged: true,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %s, want %s: %v", status.Code(err), codes.FailedPrecondition, err)
	}
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}
}

func TestBackgroundDrainConstructorsSeparateHostStateOwnership(t *testing.T) {
	t.Run("ordinary_service_is_disabled_and_host_safe", func(t *testing.T) {
		home := t.TempDir()
		stateRoot := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_STATE_HOME", stateRoot)
		if err := EnsureDataDir(); err != nil {
			t.Fatalf("EnsureDataDir: %v", err)
		}
		seedMalformedBackgroundState(t, stateRoot)

		svc, err := NewService(t.Context())
		if err != nil {
			t.Fatalf("NewService read host ACP state: %v", err)
		}
		t.Cleanup(svc.Close)
		_, err = svc.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
			SessionId: "session-1", Profile: "codex", Acknowledged: true,
		})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("code = %s, want disabled %s: %v", status.Code(err), codes.FailedPrecondition, err)
		}
	})

	t.Run("daemon_service_resumes_and_rejects_malformed_state", func(t *testing.T) {
		home := t.TempDir()
		stateRoot := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_STATE_HOME", stateRoot)
		if err := EnsureDataDir(); err != nil {
			t.Fatalf("EnsureDataDir: %v", err)
		}
		seedMalformedBackgroundState(t, stateRoot)

		if svc, err := NewDaemonService(t.Context()); err == nil {
			svc.Close()
			t.Fatal("NewDaemonService succeeded with malformed persisted background state")
		}
	})
}

func TestBackgroundDrainE2EHarnessIsDisabled(t *testing.T) {
	harness := newE2EHarness(t)
	_, err := harness.Client.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
		SessionId: "session-1", Profile: "codex", Acknowledged: true,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %s, want disabled %s: %v", status.Code(err), codes.FailedPrecondition, err)
	}
}

func TestServiceShutdownWaitsForACPBackgroundDrainWorkerWithoutDisablingPolicy(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 14, 9, 10, 0, 0, time.UTC)
	sessions := acpclient.NewStore(filepath.Join(dir, "sessions.json"))
	if err := sessions.Upsert(acpclient.SessionRecord{ID: "session-1", Status: acpclient.SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	manager := acpclient.NewBackgroundManager(
		sessions,
		acpclient.NewBackgroundStore(filepath.Join(dir, "background.json")),
		acpclient.NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		acpclient.BackgroundManagerOptions{
			Context: t.Context(),
			Now:     func() time.Time { return now },
			Resolver: func(string) (acpclient.ResolvedBackgroundProfile, error) {
				return acpclient.ResolvedBackgroundProfile{
					Spec:           acpclient.AgentSpec{Name: "fixture", Command: "fixture"},
					DescriptorHash: "descriptor-hash",
					TrustValid:     true,
				}, nil
			},
			Watcher: func(ctx context.Context, _ *acpclient.Store, _ acpclient.AgentSpec, _ acpclient.RunOptions, _ string, _ acpclient.WatchOptions, _ func(acpclient.WatchCycle)) (acpclient.WatchResult, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				<-release
				return acpclient.WatchResult{}, ctx.Err()
			},
		},
	)
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		manager.Shutdown()
	})
	svc := &Service{acpBackground: manager}
	if _, err := svc.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
		SessionId: "session-1", Profile: "fixture", Acknowledged: true,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	<-started
	shutdownDone := make(chan error, 1)
	go func() {
		_, err := svc.Shutdown(t.Context(), &pb.Empty{})
		shutdownDone <- err
	}()
	select {
	case <-canceled:
	case err := <-shutdownDone:
		t.Fatalf("Service.Shutdown returned before canceling background worker: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Service.Shutdown did not cancel background worker")
	}
	close(release)
	released = true
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Service.Shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Service.Shutdown did not wait for background worker")
	}
	policy, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if !policy.Enabled || policy.State != acpclient.BackgroundStateRunning {
		t.Fatalf("policy = %#v, want enabled/running for restart", policy)
	}
}

func backgroundDrainTestStatus(sessionID, profile, state, outcome string, now time.Time) acpclient.BackgroundStatus {
	return acpclient.BackgroundStatus{
		SessionID:      sessionID,
		Profile:        profile,
		DescriptorHash: "pinned-hash",
		State:          state,
		Outcome:        outcome,
		AcknowledgedAt: now,
		StartedAt:      now.Add(time.Second),
		UpdatedAt:      now.Add(2 * time.Second),
	}
}

func assertBackgroundDrainProto(t *testing.T, got *pb.ACPBackgroundDrain, sessionID, profile, state, outcome string, now time.Time) {
	t.Helper()
	if got.GetSessionId() != sessionID || got.GetProfile() != profile || got.GetPinnedHash() != "pinned-hash" || got.GetState() != state || got.GetLastOutcome() != outcome {
		t.Fatalf("drain = %#v", got)
	}
	if !got.GetAcknowledgedAt().AsTime().Equal(now) || !got.GetStartedAt().AsTime().Equal(now.Add(time.Second)) || !got.GetUpdatedAt().AsTime().Equal(now.Add(2*time.Second)) {
		t.Fatalf("drain timestamps = %v/%v/%v", got.GetAcknowledgedAt(), got.GetStartedAt(), got.GetUpdatedAt())
	}
}

type fakeACPBackgroundDrainManager struct {
	start    func(string, string, bool) (acpclient.BackgroundStatus, error)
	stop     func(string) (acpclient.BackgroundStatus, error)
	get      func(string) (acpclient.BackgroundStatus, error)
	list     func() ([]acpclient.BackgroundStatus, error)
	shutdown func()
}

func seedMalformedBackgroundState(t *testing.T, stateRoot string) {
	t.Helper()
	dir := filepath.Join(stateRoot, "ratchet", "acp-client")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create ACP state directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "background.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("seed malformed background state: %v", err)
	}
}

func (m *fakeACPBackgroundDrainManager) Start(sessionID, profile string, acknowledged bool) (acpclient.BackgroundStatus, error) {
	if m.start == nil {
		return acpclient.BackgroundStatus{}, errors.New("unexpected start")
	}
	return m.start(sessionID, profile, acknowledged)
}

func (m *fakeACPBackgroundDrainManager) Stop(sessionID string) (acpclient.BackgroundStatus, error) {
	if m.stop == nil {
		return acpclient.BackgroundStatus{}, errors.New("unexpected stop")
	}
	return m.stop(sessionID)
}

func (m *fakeACPBackgroundDrainManager) Get(sessionID string) (acpclient.BackgroundStatus, error) {
	if m.get == nil {
		return acpclient.BackgroundStatus{}, errors.New("unexpected get")
	}
	return m.get(sessionID)
}

func (m *fakeACPBackgroundDrainManager) List() ([]acpclient.BackgroundStatus, error) {
	if m.list == nil {
		return nil, errors.New("unexpected list")
	}
	return m.list()
}

func (m *fakeACPBackgroundDrainManager) Shutdown() {
	if m.shutdown != nil {
		m.shutdown()
	}
}
