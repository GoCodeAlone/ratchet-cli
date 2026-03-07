package daemon

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type Service struct {
	pb.UnimplementedRatchetDaemonServer
	startedAt time.Time
	engine    *EngineContext
	sessions  *SessionManager
}

func NewService(ctx context.Context) (*Service, error) {
	engine, err := NewEngineContext(ctx, DBPath())
	if err != nil {
		return nil, err
	}
	return &Service{
		startedAt: time.Now(),
		engine:    engine,
		sessions:  NewSessionManager(engine.DB),
	}, nil
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
	// TODO: graceful shutdown
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
	return status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) RespondToPermission(ctx context.Context, req *pb.PermissionResponse) (*pb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) AddProvider(ctx context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) ListProviders(ctx context.Context, _ *pb.Empty) (*pb.ProviderList, error) {
	return &pb.ProviderList{}, nil
}

func (s *Service) TestProvider(ctx context.Context, req *pb.TestProviderReq) (*pb.TestProviderResult, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) RemoveProvider(ctx context.Context, req *pb.RemoveProviderReq) (*pb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) SetDefaultProvider(ctx context.Context, req *pb.SetDefaultProviderReq) (*pb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) ListAgents(ctx context.Context, _ *pb.Empty) (*pb.AgentList, error) {
	return &pb.AgentList{}, nil
}

func (s *Service) GetAgentStatus(ctx context.Context, req *pb.AgentStatusReq) (*pb.Agent, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) StartTeam(req *pb.StartTeamReq, stream pb.RatchetDaemon_StartTeamServer) error {
	return status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *Service) GetTeamStatus(ctx context.Context, req *pb.TeamStatusReq) (*pb.TeamStatus, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}
