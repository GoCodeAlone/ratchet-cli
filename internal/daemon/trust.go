package daemon

import (
	"context"
	"strings"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func trustRuleToProto(rule policy.TrustRule) *pb.TrustRule {
	scope := rule.Scope
	if scope == "" {
		scope = "global"
	}
	return &pb.TrustRule{
		Pattern: rule.Pattern,
		Action:  string(rule.Action),
		Scope:   scope,
	}
}

func cloneTrustRules(in []policy.TrustRule) []policy.TrustRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]policy.TrustRule, len(in))
	copy(out, in)
	return out
}
