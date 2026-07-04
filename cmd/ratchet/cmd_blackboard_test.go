package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type fakeBlackboardClient struct {
	readResp *pb.BlackboardReadResp
	listResp *pb.BlackboardListResp
	written  *pb.BlackboardEntry
	err      error
}

func (f *fakeBlackboardClient) BlackboardRead(_ context.Context, section, key string) (*pb.BlackboardReadResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.readResp != nil {
		return f.readResp, nil
	}
	return &pb.BlackboardReadResp{Found: true, Entry: &pb.BlackboardEntry{
		Section: section,
		Key:     key,
		Value:   "ready",
		Author:  "tester",
	}}, nil
}

func (f *fakeBlackboardClient) BlackboardWrite(_ context.Context, section, key, value, author string) (*pb.BlackboardEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.written = &pb.BlackboardEntry{
		Section:  section,
		Key:      key,
		Value:    value,
		Author:   author,
		Revision: 7,
	}
	return f.written, nil
}

func (f *fakeBlackboardClient) BlackboardList(_ context.Context, section string) (*pb.BlackboardListResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.listResp != nil {
		return f.listResp, nil
	}
	if section == "" {
		return &pb.BlackboardListResp{Sections: []string{"coordination", "status"}}, nil
	}
	return &pb.BlackboardListResp{Entries: []*pb.BlackboardEntry{
		{Section: section, Key: "status", Value: "ready", Author: "tester", Revision: 3},
	}}, nil
}

func TestHandleBlackboardWritePrintsRevision(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"write", "coordination", "status", "ready", "--author", "agent-a"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("runBlackboard: %v", err)
	}
	if fake.written == nil {
		t.Fatal("expected write call")
	}
	if fake.written.Value != "ready" || fake.written.Author != "agent-a" {
		t.Fatalf("written = %#v", fake.written)
	}
	if got := stdout.String(); !strings.Contains(got, "coordination/status") || !strings.Contains(got, "rev=7") {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHandleBlackboardReadPrintsValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"read", "coordination", "status"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("runBlackboard: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "ready") || strings.Contains(got, "{") {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHandleBlackboardListPrintsSectionsAndEntries(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"list"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("list sections: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "coordination") || !strings.Contains(got, "status") {
		t.Fatalf("sections stdout = %q", got)
	}

	stdout.Reset()
	if err := runBlackboard(context.Background(), fake, []string{"list", "coordination"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "status") || !strings.Contains(got, "ready") {
		t.Fatalf("entries stdout = %q", got)
	}
}

func TestHandleBlackboardJSONOutput(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"read", "coordination", "status", "--json"}, &stdout, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("runBlackboard: %v", err)
	}
	var payload pb.BlackboardReadResp
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode json %q: %v", stdout.String(), err)
	}
	if !payload.Found || payload.Entry.GetValue() != "ready" {
		t.Fatalf("payload = %#v", &payload)
	}
}

func TestHandleBlackboardValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing subcommand", args: nil, want: "usage: ratchet blackboard"},
		{name: "read missing key", args: []string{"read", "coordination"}, want: "usage: ratchet blackboard read"},
		{name: "write missing value", args: []string{"write", "coordination", "status"}, want: "usage: ratchet blackboard write"},
		{name: "unknown command", args: []string{"remove", "coordination", "status"}, want: "unknown blackboard command"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			err := runBlackboard(context.Background(), &fakeBlackboardClient{}, tc.args, &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.want)
			}
		})
	}
}
