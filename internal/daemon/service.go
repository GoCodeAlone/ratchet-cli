package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	id := uuid.New().String()
	secretName := fmt.Sprintf("provider_%s", req.Alias)

	// Store API key in file-based secrets if provided
	if req.ApiKey != "" {
		if err := s.engine.SecretsProvider.Set(ctx, secretName, req.ApiKey); err != nil {
			return nil, status.Errorf(codes.Internal, "store api key: %v", err)
		}
	}

	// Clear existing default if this is the new default
	if req.IsDefault {
		if _, err := s.engine.DB.ExecContext(ctx, `UPDATE llm_providers SET is_default = 0`); err != nil {
			return nil, status.Errorf(codes.Internal, "clear defaults: %v", err)
		}
	}

	isDefault := 0
	if req.IsDefault {
		isDefault = 1
	}
	_, err := s.engine.DB.ExecContext(ctx,
		`INSERT INTO llm_providers (id, alias, type, model, secret_name, base_url, max_tokens, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Alias, req.Type, req.Model, secretName, req.BaseUrl, req.MaxTokens, isDefault,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "insert provider: %v", err)
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
	if _, err := s.engine.DB.ExecContext(ctx, `UPDATE llm_providers SET is_default = 0`); err != nil {
		return nil, status.Errorf(codes.Internal, "clear defaults: %v", err)
	}
	if _, err := s.engine.DB.ExecContext(ctx, `UPDATE llm_providers SET is_default = 1 WHERE alias = ?`, req.Alias); err != nil {
		return nil, status.Errorf(codes.Internal, "set default: %v", err)
	}
	s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)
	return &pb.Empty{}, nil
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
