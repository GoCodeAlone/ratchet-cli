package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

func TestCheckVersion_Compatible(t *testing.T) {
	svc := &Service{}
	version.Version = "v1.0.0"

	resp, err := svc.CheckVersion(context.Background(), &pb.VersionCheckReq{
		CliVersion:      "v1.0.0",
		CliProtoVersion: ProtoVersion,
	})
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
	if !resp.Compatible {
		t.Errorf("expected compatible, got message: %s", resp.Message)
	}
	if resp.ReloadRecommended {
		t.Error("expected no reload recommended when versions match")
	}
	if resp.Message != "compatible" {
		t.Errorf("expected 'compatible', got %q", resp.Message)
	}
}

func TestCheckVersion_ReloadRecommended(t *testing.T) {
	svc := &Service{}
	version.Version = "v1.1.0" // daemon is newer

	resp, err := svc.CheckVersion(context.Background(), &pb.VersionCheckReq{
		CliVersion:      "v1.0.0", // CLI is older
		CliProtoVersion: ProtoVersion,
	})
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
	if !resp.Compatible {
		t.Errorf("same proto version should be compatible")
	}
	if !resp.ReloadRecommended {
		t.Error("expected reload recommended when binary versions differ")
	}
	if resp.DaemonVersion != "v1.1.0" {
		t.Errorf("DaemonVersion: got %q, want v1.1.0", resp.DaemonVersion)
	}
}

func TestCheckVersion_Incompatible(t *testing.T) {
	svc := &Service{}
	version.Version = "v2.0.0"

	resp, err := svc.CheckVersion(context.Background(), &pb.VersionCheckReq{
		CliVersion:      "v2.0.0",
		CliProtoVersion: ProtoVersion + 1, // CLI has newer proto
	})
	if err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
	if resp.Compatible {
		t.Error("expected incompatible when proto versions differ")
	}
	if resp.ReloadRecommended {
		t.Error("should not recommend reload when incompatible (need full restart)")
	}
}
