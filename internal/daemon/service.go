package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

// ProtoVersion is the current protocol version. Increment this when making
// breaking proto changes (removing/renaming fields or RPCs). Minor additions
// such as new fields or new RPCs do not require a bump.
const ProtoVersion = 1

type Service struct {
	pb.UnimplementedRatchetDaemonServer
	startedAt    time.Time
	engine       *EngineContext
	sessions     *SessionManager
	permGate     *permissionGate
	approvalGate *ApprovalGate
	plans        *PlanManager
	cron         *CronScheduler
	fleet        *FleetManager
	teams        *TeamManager
	tokens       *TokenTracker
	jobs         *JobRegistry
	broadcaster  *SessionBroadcaster
	shutdownFn   func()
	meshBB       *mesh.Blackboard
	meshRouter   *mesh.Router
}

func NewService(ctx context.Context) (*Service, error) {
	// Publish version so checkpoint and health can reference it.
	daemonVersion = version.Version

	engine, err := NewEngineContext(ctx, DBPath())
	if err != nil {
		return nil, err
	}
	sm := NewSessionManager(engine.DB)
	// Clean up stale sessions from previous daemon runs (24h expiry).
	if cleaned, err := sm.CleanupStale(ctx, 24*time.Hour); err != nil {
		log.Printf("session cleanup: %v", err)
	} else if cleaned > 0 {
		log.Printf("session cleanup: marked %d stale sessions as completed", cleaned)
	}

	svc := &Service{
		startedAt:    time.Now(),
		engine:       engine,
		sessions:     sm,
		permGate:     newPermissionGate(),
		approvalGate: NewApprovalGate(),
		plans:        NewPlanManager(engine.Hooks),
	}
	cfg, _ := config.Load()
	routing := config.ModelRouting{}
	if cfg != nil {
		routing = cfg.ModelRouting
	}
	svc.fleet = NewFleetManager(routing, engine, engine.Hooks)
	svc.teams = NewTeamManager(engine, engine.Hooks)
	svc.tokens = NewTokenTracker()
	svc.jobs = NewJobRegistry()
	svc.broadcaster = NewSessionBroadcaster()
	svc.meshBB = mesh.NewBlackboard()
	svc.meshRouter = mesh.NewRouter()

	// Create and start cron scheduler AFTER all Service fields are initialized,
	// since tick callbacks invoke svc.handleChat which depends on svc.tokens etc.
	svc.cron = NewCronScheduler(engine.DB, func(sessionID, command string) {
		go func() {
			tickCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if engine.Hooks != nil {
				_ = engine.Hooks.Run(hooks.OnCronTick, map[string]string{"session_id": sessionID, "command": command})
			}
			ns := &noopSendServer{ctx: tickCtx}
			if err := svc.handleChat(tickCtx, sessionID, command, ns); err != nil {
				log.Printf("cron tick session=%s command=%q: %v", sessionID, command, err)
			}
		}()
	})
	if err := svc.cron.Start(ctx); err != nil {
		engine.Close()
		return nil, fmt.Errorf("start cron scheduler: %w", err)
	}
	svc.jobs.Register("session", NewSessionJobProvider(svc.sessions))
	svc.jobs.Register("fleet_worker", NewFleetJobProvider(svc.fleet))
	svc.jobs.Register("team_agent", NewTeamJobProvider(svc.teams))
	svc.jobs.Register("cron", NewCronJobProvider(svc.cron))

	// Restore state from checkpoint if one exists (written during graceful reload).
	if cp, err := LoadCheckpoint(); err == nil {
		log.Printf("restoring from checkpoint (daemon %s)", cp.Version)
		os.Remove(CheckpointPath()) // consume immediately so it doesn't linger
	}

	return svc, nil
}

func (s *Service) Health(ctx context.Context, _ *pb.Empty) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Healthy:        true,
		ActiveSessions: 0,
		ActiveAgents:   0,
		Uptime:         time.Since(s.startedAt).Round(time.Second).String(),
		Version:        version.Version,
		Commit:         version.Commit,
		ProtoVersion:   ProtoVersion,
	}, nil
}

// CheckVersion compares CLI version/proto against the running daemon and
// returns compatibility information.
func (s *Service) CheckVersion(ctx context.Context, req *pb.VersionCheckReq) (*pb.VersionCheckResp, error) {
	daemonVer := version.Version
	compatible := req.CliProtoVersion == ProtoVersion
	reloadRecommended := req.CliVersion != daemonVer && compatible

	var msg string
	switch {
	case !compatible:
		msg = fmt.Sprintf("protocol mismatch: CLI proto v%d, daemon proto v%d — please restart daemon",
			req.CliProtoVersion, ProtoVersion)
	case reloadRecommended:
		msg = fmt.Sprintf("version mismatch: CLI %s, daemon %s — reload recommended",
			req.CliVersion, daemonVer)
	default:
		msg = "compatible"
	}

	return &pb.VersionCheckResp{
		Compatible:        compatible,
		ReloadRecommended: reloadRecommended,
		DaemonVersion:     daemonVer,
		Message:           msg,
	}, nil
}

// RequestReload checkpoints state and initiates a graceful daemon restart.
// It streams status events back to the caller.
func (s *Service) RequestReload(req *pb.ReloadReq, stream pb.RatchetDaemon_RequestReloadServer) error {
	_ = stream.Send(&pb.ReloadStatus{Status: "checkpointing", Message: "saving daemon state..."})

	cp, err := ExportCheckpoint(s)
	if err != nil {
		_ = stream.Send(&pb.ReloadStatus{Status: "failed", Message: fmt.Sprintf("checkpoint failed: %v", err)})
		return status.Errorf(codes.Internal, "checkpoint: %v", err)
	}
	if err := SaveCheckpoint(cp); err != nil {
		_ = stream.Send(&pb.ReloadStatus{Status: "failed", Message: fmt.Sprintf("save checkpoint failed: %v", err)})
		return status.Errorf(codes.Internal, "save checkpoint: %v", err)
	}

	_ = stream.Send(&pb.ReloadStatus{Status: "restarting", Message: "checkpoint saved, restarting daemon..."})

	// Trigger reload asynchronously so the stream response can flush first.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("reload: panic: %v", r)
			}
		}()
		if err := TriggerReload(); err != nil {
			log.Printf("reload trigger failed: %v", err)
		}
	}()

	return nil
}

// SetShutdownFunc injects the cancel function that shuts down the daemon.
// Called by daemon main after NewService returns.
func (s *Service) SetShutdownFunc(fn func()) {
	s.shutdownFn = fn
}

func (s *Service) Shutdown(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
	if s.shutdownFn != nil {
		go func() {
			time.Sleep(100 * time.Millisecond) // let RPC response flush
			s.shutdownFn()
		}()
	}
	return &pb.Empty{}, nil
}

func (s *Service) CreateSession(ctx context.Context, req *pb.CreateSessionReq) (*pb.Session, error) {
	si, err := s.sessions.Create(ctx, req.WorkingDir, req.Provider, req.Model, req.InitialPrompt)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}
	return &pb.Session{
		Id:         si.ID,
		Name:       si.Name,
		Status:     si.Status,
		WorkingDir: si.WorkingDir,
		Provider:   si.Provider,
		Model:      si.Model,
	}, nil
}

func (s *Service) ListSessions(ctx context.Context, _ *pb.Empty) (*pb.SessionList, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list sessions: %v", err)
	}
	var pbSessions []*pb.Session
	for _, si := range sessions {
		pbSessions = append(pbSessions, &pb.Session{
			Id:         si.ID,
			Name:       si.Name,
			Status:     si.Status,
			WorkingDir: si.WorkingDir,
			Provider:   si.Provider,
			Model:      si.Model,
		})
	}
	return &pb.SessionList{Sessions: pbSessions}, nil
}

func (s *Service) AttachSession(req *pb.AttachReq, stream pb.RatchetDaemon_AttachSessionServer) error {
	// Verify the session exists before subscribing (prevents hanging on nonexistent sessions).
	if s.sessions != nil {
		if _, err := s.sessions.Get(stream.Context(), req.SessionId); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return status.Errorf(codes.NotFound, "session %s not found", req.SessionId)
			}
			return status.Errorf(codes.Internal, "lookup session %s: %v", req.SessionId, err)
		}
	}
	ch, subID := s.broadcaster.Subscribe(req.SessionId)
	defer s.broadcaster.Unsubscribe(req.SessionId, subID)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

func (s *Service) DetachSession(ctx context.Context, req *pb.DetachReq) (*pb.Empty, error) {
	// Detach is handled client-side by cancelling the AttachSession stream.
	return &pb.Empty{}, nil
}

func (s *Service) KillSession(ctx context.Context, req *pb.KillReq) (*pb.Empty, error) {
	if err := s.sessions.Kill(ctx, req.SessionId); err != nil {
		return nil, status.Errorf(codes.Internal, "kill session: %v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) SendMessage(req *pb.SendMessageReq, stream pb.RatchetDaemon_SendMessageServer) error {
	return s.handleChat(stream.Context(), req.SessionId, req.Content, stream)
}

func (s *Service) RespondToPermission(ctx context.Context, req *pb.PermissionResponse) (*pb.Empty, error) {
	if s.permGate.Respond(req) {
		return &pb.Empty{}, nil
	}
	if s.approvalGate.Resolve(req.RequestId, req.Allowed, req.Scope) {
		return &pb.Empty{}, nil
	}
	return nil, status.Error(codes.NotFound, "no pending permission request with that ID")
}

func (s *Service) AddProvider(ctx context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
	id := uuid.New().String()
	// Only create a secret reference if an API key is provided.
	// Providers like Ollama don't need keys — storing an empty secret_name
	// prevents the ProviderRegistry from trying to resolve a nonexistent secret.
	secretName := ""
	if req.ApiKey != "" {
		secretName = fmt.Sprintf("provider_%s", req.Alias)
	}

	isDefault := 0
	if req.IsDefault {
		isDefault = 1
	}

	tx, err := s.engine.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Clear existing default if this is the new default
	if req.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE llm_providers SET is_default = 0`); err != nil {
			return nil, status.Errorf(codes.Internal, "clear defaults: %v", err)
		}
	}

	// Apply server-side default for max_tokens when the client omits it
	// (protobuf default is 0, but providers expect a reasonable value).
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// DB insert before secret store to avoid orphaned secrets on constraint failure
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO llm_providers (id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Alias, req.Type, req.Model, secretName, req.BaseUrl, maxTokens, "{}", isDefault,
	); err != nil {
		return nil, status.Errorf(codes.Internal, "insert provider: %v", err)
	}

	// Store API key after successful insert
	if req.ApiKey != "" {
		if err := s.engine.SecretsProvider.Set(ctx, secretName, req.ApiKey); err != nil {
			return nil, status.Errorf(codes.Internal, "store api key: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)

	return &pb.Provider{
		Alias:     req.Alias,
		Type:      req.Type,
		Model:     req.Model,
		BaseUrl:   req.BaseUrl,
		IsDefault: req.IsDefault,
	}, nil
}

func (s *Service) ListProviders(ctx context.Context, _ *pb.Empty) (*pb.ProviderList, error) {
	rows, err := s.engine.DB.QueryContext(ctx,
		`SELECT alias, type, model, base_url, is_default FROM llm_providers ORDER BY alias`,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list providers: %v", err)
	}
	defer rows.Close()

	var providers []*pb.Provider
	for rows.Next() {
		var p pb.Provider
		var isDefault int
		if err := rows.Scan(&p.Alias, &p.Type, &p.Model, &p.BaseUrl, &isDefault); err != nil {
			return nil, status.Errorf(codes.Internal, "scan provider: %v", err)
		}
		p.IsDefault = isDefault == 1
		providers = append(providers, &p)
	}
	return &pb.ProviderList{Providers: providers}, rows.Err()
}

func (s *Service) TestProvider(ctx context.Context, req *pb.TestProviderReq) (*pb.TestProviderResult, error) {
	ok, msg, latency, err := s.engine.ProviderRegistry.TestConnection(ctx, req.Alias)
	if err != nil {
		return &pb.TestProviderResult{Success: false, Message: err.Error()}, nil
	}
	return &pb.TestProviderResult{
		Success:   ok,
		Message:   msg,
		LatencyMs: latency.Milliseconds(),
	}, nil
}

func (s *Service) RemoveProvider(ctx context.Context, req *pb.RemoveProviderReq) (*pb.Empty, error) {
	if _, err := s.engine.DB.ExecContext(ctx, `DELETE FROM llm_providers WHERE alias = ?`, req.Alias); err != nil {
		return nil, status.Errorf(codes.Internal, "delete provider: %v", err)
	}
	s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)
	return &pb.Empty{}, nil
}

func (s *Service) SetDefaultProvider(ctx context.Context, req *pb.SetDefaultProviderReq) (*pb.Empty, error) {
	tx, err := s.engine.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE llm_providers SET is_default = 0`); err != nil {
		return nil, status.Errorf(codes.Internal, "clear defaults: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE llm_providers SET is_default = 1 WHERE alias = ?`, req.Alias); err != nil {
		return nil, status.Errorf(codes.Internal, "set default: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}
	s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)
	return &pb.Empty{}, nil
}

func (s *Service) ListAgents(ctx context.Context, _ *pb.Empty) (*pb.AgentList, error) {
	agents := s.teams.ListAllAgents()
	agents = append(agents, s.fleet.ListAllWorkers()...)
	return &pb.AgentList{Agents: agents}, nil
}

func (s *Service) GetAgentStatus(ctx context.Context, req *pb.AgentStatusReq) (*pb.Agent, error) {
	if ag := s.teams.FindAgent(req.AgentId); ag != nil {
		return ag, nil
	}
	if ag := s.fleet.FindWorker(req.AgentId); ag != nil {
		return ag, nil
	}
	return nil, status.Errorf(codes.NotFound, "agent %s not found", req.AgentId)
}

func (s *Service) UpdateProviderModel(ctx context.Context, req *pb.UpdateProviderModelReq) (*pb.Empty, error) {
	result, err := s.engine.DB.ExecContext(ctx, "UPDATE llm_providers SET model = ? WHERE alias = ?", req.Model, req.Alias)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update model: %v", err)
	}
	rows, raErr := result.RowsAffected()
	if raErr != nil {
		return nil, status.Errorf(codes.Internal, "rows affected: %v", raErr)
	}
	if rows == 0 {
		return nil, status.Errorf(codes.NotFound, "provider %q not found", req.Alias)
	}
	s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)
	return &pb.Empty{}, nil
}

func (s *Service) StartTeam(req *pb.StartTeamReq, stream pb.RatchetDaemon_StartTeamServer) error {
	teamID, eventCh := s.teams.StartTeam(stream.Context(), req)

	// Only emit the team ID event when a real team was successfully created.
	// An empty teamID means creation failed; the error event will be streamed
	// below from eventCh, so we don't want to mislead clients with a fake team.
	if teamID != "" {
		if err := stream.Send(&pb.TeamEvent{
			Event: &pb.TeamEvent_AgentSpawned{
				AgentSpawned: &pb.AgentSpawned{
					AgentId:   teamID,
					AgentName: "__team__",
					Role:      "team",
				},
			},
		}); err != nil {
			return err
		}
	}

	for ev := range eventCh {
		if err := stream.Send(ev); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) GetTeamStatus(ctx context.Context, req *pb.TeamStatusReq) (*pb.TeamStatus, error) {
	st, err := s.teams.GetStatus(req.TeamId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get team status: %v", err)
	}
	return st, nil
}

func (s *Service) CreateCron(ctx context.Context, req *pb.CreateCronReq) (*pb.CronJob, error) {
	j, err := s.cron.Create(ctx, req.SessionId, req.Schedule, req.Command)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "create cron: %v", err)
	}
	return cronJobToPB(j), nil
}

func (s *Service) ListCrons(ctx context.Context, _ *pb.Empty) (*pb.CronJobList, error) {
	jobs, err := s.cron.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list crons: %v", err)
	}
	var pbJobs []*pb.CronJob
	for _, j := range jobs {
		pbJobs = append(pbJobs, cronJobToPB(j))
	}
	return &pb.CronJobList{Jobs: pbJobs}, nil
}

func (s *Service) PauseCron(ctx context.Context, req *pb.CronJobReq) (*pb.Empty, error) {
	if err := s.cron.Pause(ctx, req.JobId); err != nil {
		return nil, status.Errorf(codes.NotFound, "pause cron: %v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) ResumeCron(ctx context.Context, req *pb.CronJobReq) (*pb.Empty, error) {
	if err := s.cron.Resume(ctx, req.JobId); err != nil {
		return nil, status.Errorf(codes.NotFound, "resume cron: %v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) StopCron(ctx context.Context, req *pb.CronJobReq) (*pb.Empty, error) {
	if err := s.cron.Stop(ctx, req.JobId); err != nil {
		return nil, status.Errorf(codes.Internal, "stop cron: %v", err)
	}
	return &pb.Empty{}, nil
}

// StartFleet starts a fleet of workers for plan execution and streams status events.
func (s *Service) StartFleet(req *pb.StartFleetReq, stream pb.RatchetDaemon_StartFleetServer) error {
	// Decompose plan into step descriptions. Fall back to single step if plan not found.
	var steps []string
	if plan := s.plans.Get(req.PlanId); plan != nil {
		for _, step := range plan.Steps {
			if step.Status != "skipped" {
				steps = append(steps, step.Description)
			}
		}
	}
	if len(steps) == 0 {
		if req.PlanId != "" {
			steps = []string{req.PlanId}
		} else {
			steps = []string{"default-step"}
		}
	}

	eventCh := make(chan *pb.FleetStatus, 32)
	_ = s.fleet.StartFleet(stream.Context(), req, steps, eventCh)

	for fs := range eventCh {
		if err := stream.Send(&pb.ChatEvent{
			Event: &pb.ChatEvent_FleetStatus{FleetStatus: fs},
		}); err != nil {
			return err
		}
	}
	return nil
}

// GetFleetStatus returns the current status of a fleet.
func (s *Service) GetFleetStatus(ctx context.Context, req *pb.FleetStatusReq) (*pb.FleetStatus, error) {
	fs, err := s.fleet.GetStatus(req.FleetId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return fs, nil
}

// KillFleetWorker cancels a specific worker within a fleet.
func (s *Service) KillFleetWorker(ctx context.Context, req *pb.KillFleetWorkerReq) (*pb.Empty, error) {
	if err := s.fleet.KillWorker(req.FleetId, req.WorkerId); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) ListJobs(ctx context.Context, _ *pb.Empty) (*pb.JobList, error) {
	return &pb.JobList{Jobs: s.jobs.ListJobs()}, nil
}

func (s *Service) PauseJob(ctx context.Context, req *pb.JobReq) (*pb.Empty, error) {
	if err := s.jobs.PauseJob(req.JobId); err != nil {
		return nil, status.Errorf(codes.NotFound, "pause job: %v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) ResumeJob(ctx context.Context, req *pb.JobReq) (*pb.Empty, error) {
	if err := s.jobs.ResumeJob(req.JobId); err != nil {
		return nil, status.Errorf(codes.NotFound, "resume job: %v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) KillJob(ctx context.Context, req *pb.JobReq) (*pb.Empty, error) {
	if err := s.jobs.KillJob(req.JobId); err != nil {
		return nil, status.Errorf(codes.NotFound, "kill job: %v", err)
	}
	return &pb.Empty{}, nil
}

func cronJobToPB(j CronJob) *pb.CronJob {
	return &pb.CronJob{
		Id:        j.ID,
		SessionId: j.SessionID,
		Schedule:  j.Schedule,
		Command:   j.Command,
		Status:    j.Status,
		LastRun:   j.LastRun,
		NextRun:   j.NextRun,
		RunCount:  j.RunCount,
	}
}

// ApprovePlan implements the ApprovePlan RPC.
func (s *Service) ApprovePlan(req *pb.ApprovePlanReq, stream pb.RatchetDaemon_ApprovePlanServer) error {
	if err := s.plans.Approve(req.PlanId, req.SkipSteps); err != nil {
		return status.Errorf(codes.InvalidArgument, "approve plan: %v", err)
	}
	plan := s.plans.Get(req.PlanId)
	if plan == nil {
		return status.Error(codes.NotFound, "plan not found after approval")
	}
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_PlanProposed{
			PlanProposed: plan,
		},
	})
}

// RejectPlan implements the RejectPlan RPC.
func (s *Service) RejectPlan(ctx context.Context, req *pb.RejectPlanReq) (*pb.Empty, error) {
	if err := s.plans.Reject(req.PlanId, req.Feedback); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "reject plan: %v", err)
	}
	return &pb.Empty{}, nil
}

// RegisterMeshNode registers a remote node in the service mesh and returns its generated ID.
// The returned ID should be used by the client when opening a MeshStream (sent as
// the first MeshEvent.node_registered message) so both sides agree on the node identity.
func (s *Service) RegisterMeshNode(ctx context.Context, req *pb.RegisterNodeReq) (*pb.RegisterNodeResp, error) {
	nodeID := uuid.New().String()
	// Don't register with router here — MeshStream does that using the same ID
	// sent by the client in the handshake, ensuring a single consistent identity.
	return &pb.RegisterNodeResp{NodeId: nodeID}, nil
}

// MeshStream handles bidirectional mesh event exchange with a remote daemon node.
//
// Protocol: The first message from the client should be a node_registered event
// carrying the nodeID from RegisterMeshNode. If missing, a new ID is generated.
//
// All outgoing sends are serialized through a single channel to avoid concurrent
// stream.Send calls. Both AgentMessages and BlackboardSync events are forwarded.
func (s *Service) MeshStream(stream pb.RatchetDaemon_MeshStreamServer) error {
	ctx := stream.Context()

	// Handshake: read first message to get node ID.
	nodeID := uuid.New().String()
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	processFirstAsEvent := false
	if nr, ok := first.Event.(*pb.MeshEvent_NodeRegistered); ok {
		nodeID = nr.NodeRegistered.NodeId
	} else {
		// Not a handshake — process it as a regular event after registration.
		processFirstAsEvent = true
	}

	inbox, regErr := s.meshRouter.Register(nodeID)
	if regErr != nil {
		return status.Errorf(codes.Internal, "register mesh stream node %s: %v", nodeID, regErr)
	}
	defer s.meshRouter.Unregister(nodeID)

	// Process the first message if it wasn't a handshake.
	if processFirstAsEvent {
		s.handleMeshEvent(first)
	}

	// sendCh serializes all outgoing events to avoid concurrent stream.Send.
	sendCh := make(chan *pb.MeshEvent, 64)
	sendErrCh := make(chan error, 1)

	// Sender goroutine.
	go func() {
		for {
			select {
			case ev, ok := <-sendCh:
				if !ok {
					return
				}
				if err := stream.Send(ev); err != nil {
					select {
					case sendErrCh <- err:
					default:
					}
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Watch local blackboard for writes → forward to remote via sendCh.
	watcherID := s.meshBB.Watch(func(key string, val mesh.Entry) {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return
		}
		valueBytes, _ := json.Marshal(val.Value)
		select {
		case sendCh <- &pb.MeshEvent{
			Event: &pb.MeshEvent_BlackboardSync{
				BlackboardSync: &pb.BlackboardSync{
					Section:  parts[0],
					Key:      parts[1],
					Value:    valueBytes,
					Author:   val.Author,
					Revision: val.Revision,
				},
			},
		}:
		default:
		}
	})
	defer s.meshBB.Unwatch(watcherID)

	// Forward router inbox messages to the remote node via sendCh.
	go func() {
		for {
			select {
			case msg, ok := <-inbox:
				if !ok {
					return
				}
				select {
				case sendCh <- &pb.MeshEvent{
					Event: &pb.MeshEvent_AgentMessage{
						AgentMessage: &pb.AgentMessage{
							FromAgent: msg.From,
							ToAgent:   msg.To,
							Content:   msg.Content,
						},
					},
				}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Main loop: receive events from the remote node.
	for {
		select {
		case sendErr := <-sendErrCh:
			return fmt.Errorf("mesh stream send: %w", sendErr)
		default:
		}

		ev, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		s.handleMeshEvent(ev)
	}
}

// handleMeshEvent processes a single incoming MeshEvent from a remote node.
func (s *Service) handleMeshEvent(ev *pb.MeshEvent) {
	switch e := ev.Event.(type) {
	case *pb.MeshEvent_BlackboardSync:
		sync := e.BlackboardSync
		var value any
		_ = json.Unmarshal(sync.Value, &value)
		// Use WriteFromRemote to avoid echo loops.
		s.meshBB.WriteFromRemote(sync.Section, sync.Key, value, sync.Author, sync.Revision)
	case *pb.MeshEvent_AgentMessage:
		msg := e.AgentMessage
		_ = s.meshRouter.Send(mesh.Message{
			From:    msg.FromAgent,
			To:      msg.ToAgent,
			Content: msg.Content,
			Type:    "result",
		})
	}
}

// noopSendServer discards all events; used for cron tick injection where no gRPC client is connected.
type noopSendServer struct{ ctx context.Context }

func (n *noopSendServer) Send(*pb.ChatEvent) error            { return nil }
func (n *noopSendServer) Context() context.Context           { return n.ctx }
func (n *noopSendServer) SetHeader(metadata.MD) error        { return nil }
func (n *noopSendServer) SendHeader(metadata.MD) error       { return nil }
func (n *noopSendServer) SetTrailer(metadata.MD)             {}
func (n *noopSendServer) SendMsg(any) error                  { return nil }
func (n *noopSendServer) RecvMsg(any) error                  { return nil }
