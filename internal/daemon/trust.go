package daemon

import (
	"context"
	"strings"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) GetTrustState(context.Context, *pb.Empty) (*pb.TrustState, error) {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	return s.trustStateLocked(), nil
}

func (s *Service) SetTrustMode(_ context.Context, req *pb.SetTrustModeReq) (*pb.TrustState, error) {
	mode := strings.TrimSpace(req.GetMode())
	if !validTrustMode(mode) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid trust mode %q", req.GetMode())
	}

	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.rebuildTrustEngineLocked(mode)
	return s.trustStateLocked(), nil
}

func (s *Service) AddTrustRule(_ context.Context, req *pb.AddTrustRuleReq) (*pb.TrustState, error) {
	pattern := strings.TrimSpace(req.GetPattern())
	if pattern == "" {
		return nil, status.Error(codes.InvalidArgument, "trust rule pattern is required")
	}
	action, ok := trustAction(req.GetAction())
	if !ok || action == policy.Ask {
		return nil, status.Errorf(codes.InvalidArgument, "invalid trust rule action %q", req.GetAction())
	}
	scope := strings.TrimSpace(req.GetScope())
	if scope == "" {
		scope = "global"
	}
	rule := policy.TrustRule{
		Pattern: pattern,
		Action:  action,
		Scope:   scope,
	}

	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.trustRuntime = append(s.trustRuntime, rule)
	if s.trustEngine == nil {
		s.rebuildTrustEngineLocked(s.currentTrustModeLocked())
	} else {
		s.trustEngine.AddRule(rule)
	}
	return s.trustStateLocked(), nil
}

func (s *Service) ResetTrust(context.Context, *pb.Empty) (*pb.TrustState, error) {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.trustRuntime = nil
	mode := s.trustDefaultMode
	if mode == "" {
		mode = "conservative"
	}
	s.rebuildTrustEngineLocked(mode)
	return s.trustStateLocked(), nil
}

func (s *Service) AddTrustGrant(_ context.Context, req *pb.AddTrustRuleReq) (*pb.TrustState, error) {
	pattern, action, scope, err := validatePersistentTrustGrant(req.GetPattern(), req.GetAction(), req.GetScope())
	if err != nil {
		return nil, err
	}

	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	if s.trustStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "persistent trust store is unavailable")
	}
	if err := s.trustStore.Grant(pattern, action, scope, "operator"); err != nil {
		return nil, status.Errorf(codes.Internal, "persist trust grant: %v", err)
	}
	return s.trustStateLocked(), nil
}

func (s *Service) RevokeTrustGrant(_ context.Context, req *pb.RevokeTrustGrantReq) (*pb.TrustState, error) {
	pattern := strings.TrimSpace(req.GetPattern())
	if pattern == "" {
		return nil, status.Error(codes.InvalidArgument, "trust grant pattern is required")
	}
	scope := normalTrustScope(req.GetScope())

	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	if s.trustStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "persistent trust store is unavailable")
	}
	if err := s.trustStore.Revoke(pattern, scope); err != nil {
		return nil, status.Errorf(codes.Internal, "revoke trust grant: %v", err)
	}
	return s.trustStateLocked(), nil
}

func (s *Service) rebuildTrustEngineLocked(mode string) {
	rules := append(cloneTrustRules(s.trustDefaults), s.trustRuntime...)
	s.trustMode = mode
	s.trustEngine = policy.NewTrustEngine(mode, rules, nil)
	if s.trustStore != nil {
		s.trustEngine.SetPermissionStore(s.trustStore)
	}
}

func (s *Service) trustStateLocked() *pb.TrustState {
	if s.trustEngine == nil {
		s.rebuildTrustEngineLocked(s.currentTrustModeLocked())
	}
	mode := s.trustEngine.Mode()
	if mode == "" {
		mode = s.currentTrustModeLocked()
	}
	state := &pb.TrustState{Mode: mode}
	for _, rule := range s.trustEngine.Rules() {
		state.Rules = append(state.Rules, trustRuleToProto(rule))
	}
	for _, grant := range s.trustGrantsLocked() {
		state.Grants = append(state.Grants, trustGrantToProto(grant))
	}
	return state
}

func (s *Service) currentTrustModeLocked() string {
	if s.trustMode != "" {
		return s.trustMode
	}
	if s.trustDefaultMode != "" {
		return s.trustDefaultMode
	}
	return "conservative"
}

func validTrustMode(mode string) bool {
	if mode == "custom" {
		return true
	}
	_, ok := policy.ModePresets[mode]
	return ok
}

func trustAction(action string) (policy.Action, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case string(policy.Allow):
		return policy.Allow, true
	case string(policy.Deny):
		return policy.Deny, true
	case string(policy.Ask):
		return policy.Ask, true
	default:
		return "", false
	}
}

func validatePersistentTrustGrant(pattern, actionText, scopeText string) (string, policy.Action, string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", "", "", status.Error(codes.InvalidArgument, "trust grant pattern is required")
	}
	action, ok := trustAction(actionText)
	if !ok || action == policy.Ask {
		return "", "", "", status.Errorf(codes.InvalidArgument, "invalid trust grant action %q", actionText)
	}
	return pattern, action, normalTrustScope(scopeText), nil
}

func normalTrustScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "global"
	}
	return scope
}

func trustRuleToProto(rule policy.TrustRule) *pb.TrustRule {
	return &pb.TrustRule{
		Pattern: rule.Pattern,
		Action:  string(rule.Action),
		Scope:   normalTrustScope(rule.Scope),
	}
}

func (s *Service) trustGrantsLocked() []policy.PermissionGrant {
	if s.trustStore == nil {
		return nil
	}
	grants, err := s.trustStore.List()
	if err != nil {
		return nil
	}
	return grants
}

func trustGrantToProto(grant policy.PermissionGrant) *pb.TrustGrant {
	out := &pb.TrustGrant{
		Id:        grant.ID,
		Pattern:   grant.Pattern,
		Action:    string(grant.Action),
		Scope:     normalTrustScope(grant.Scope),
		GrantedBy: grant.GrantedBy,
	}
	if !grant.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(grant.CreatedAt)
	}
	return out
}

func cloneTrustRules(in []policy.TrustRule) []policy.TrustRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]policy.TrustRule, len(in))
	copy(out, in)
	return out
}
