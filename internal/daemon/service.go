package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type Service struct {
	pb.UnimplementedRatchetDaemonServer
	startedAt time.Time
	engine    *EngineContext
	sessions  *SessionManager
	permGate  *permissionGate
	plans     *PlanManager
	cron      *CronScheduler
	fleet     *FleetManager
	teams     *TeamManager
}

func NewService(ctx context.Context) (*Service, error) {
	engine, err := NewEngineContext(ctx, DBPath())
	if err != nil {
		return nil, err
	}
	svc := &Service{
		startedAt: time.Now(),
		engine:    engine,
		sessions:  NewSessionManager(engine.DB),
		permGate:  newPermissionGate(),
		plans:     NewPlanManager(),
	}
	svc.cron = NewCronScheduler(engine.DB, func(sessionID, command string) {
		// Tick handler: future integration point to inject command into session.
	})
	if err := svc.cron.Start(ctx); err != nil {
		engine.Close()
		return nil, fmt.Errorf("start cron scheduler: %w", err)
	}
	cfg, _ := config.Load()
	routing := config.ModelRouting{}
	if cfg != nil {
		routing = cfg.ModelRouting
	}
	svc.fleet = NewFleetManager(routing)
	svc.teams = NewTeamManager()
	return svc, nil
}

func (s *Service) Health(ctx context.Context, _ *pb.Empty) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Healthy:        true,
		ActiveSessions: 0,
		ActiveAgents:   0,
		Uptime:         time.Since(s.startedAt).Round(time.Second).String(),
	}, nil
}

func (s *Service) Shutdown(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
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
	return status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) DetachSession(ctx context.Context, req *pb.DetachReq) (*pb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
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
	if !s.permGate.Respond(req) {
		return nil, status.Error(codes.NotFound, "no pending permission request with that ID")
	}
	return &pb.Empty{}, nil
}

func (s *Service) AddProvider(ctx context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
	id := uuid.New().String()
	secretName := fmt.Sprintf("provider_%s", req.Alias)

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

	// DB insert before secret store to avoid orphaned secrets on constraint failure
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO llm_providers (id, alias, type, model, secret_name, base_url, max_tokens, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Alias, req.Type, req.Model, secretName, req.BaseUrl, req.MaxTokens, isDefault,
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
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) GetAgentStatus(ctx context.Context, req *pb.AgentStatusReq) (*pb.Agent, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) StartTeam(req *pb.StartTeamReq, stream pb.RatchetDaemon_StartTeamServer) error {
	_, eventCh := s.teams.StartTeam(stream.Context(), req)
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
	// Decompose plan steps — for now use a simple single-step decomposition.
	// Future: load plan from PlanManager and extract independent steps.
	steps := []string{req.PlanId}
	if req.PlanId == "" {
		steps = []string{"default-step"}
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
