package main

import (
	"bufio"
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
		{Section: section, Key: "status", Value: "ready", Author: "tester", Revision: 3, Timestamp: "2026-07-04T10:00:00Z"},
	}}, nil
}

func TestHandleBlackboardWritePrintsRevision(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"write", "coordination", "status", "ready", "--author", "agent-a"}, &stdout)

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
}

func TestHandleBlackboardReadPrintsValue(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"read", "coordination", "status"}, &stdout)

	if err != nil {
		t.Fatalf("runBlackboard: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "ready") || strings.Contains(got, "{") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestHandleBlackboardListPrintsSectionsAndEntries(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"list"}, &stdout); err != nil {
		t.Fatalf("list sections: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "coordination") || !strings.Contains(got, "status") {
		t.Fatalf("sections stdout = %q", got)
	}

	stdout.Reset()
	if err := runBlackboard(context.Background(), fake, []string{"list", "coordination"}, &stdout); err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "status") || !strings.Contains(got, "ready") {
		t.Fatalf("entries stdout = %q", got)
	}
}

func TestHandleBlackboardJSONOutput(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"read", "coordination", "status", "--json"}, &stdout)

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

func TestHandleBlackboardWriteSupportsEndOfFlagsMarker(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"write", "coordination", "flaggy", "--", "--json", "--author", "literal"}, &stdout)

	if err != nil {
		t.Fatalf("runBlackboard: %v", err)
	}
	if fake.written == nil {
		t.Fatal("expected write call")
	}
	if fake.written.Value != "--json --author literal" {
		t.Fatalf("written value = %q", fake.written.Value)
	}
	if fake.written.Author == "literal" {
		t.Fatalf("--author after -- must be positional, author = %q", fake.written.Author)
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
			err := runBlackboard(context.Background(), &fakeBlackboardClient{}, tc.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.want)
			}
		})
	}
}

func TestHandleBlackboardExportSectionJSON(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	err := runBlackboard(context.Background(), fake, []string{"export", "coordination", "--json"}, &stdout)

	if err != nil {
		t.Fatalf("runBlackboard export: %v", err)
	}
	var records []blackboardExportRecord
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatalf("decode export json %q: %v", stdout.String(), err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	record := records[0]
	if record.Section != "coordination" || record.Key != "status" || record.Value != "ready" ||
		record.Author != "tester" || record.Revision != 3 || record.Timestamp != "2026-07-04T10:00:00Z" {
		t.Fatalf("record = %#v", record)
	}
	if record.Messaging.Text != "[coordination/status] ready" {
		t.Fatalf("messaging text = %q", record.Messaging.Text)
	}
}

func TestHandleBlackboardExportAllSectionsJSON(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"export", "--json"}, &stdout); err != nil {
		t.Fatalf("runBlackboard export all: %v", err)
	}
	var records []blackboardExportRecord
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatalf("decode export json %q: %v", stdout.String(), err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %#v", records)
	}
	gotSections := records[0].Section + "," + records[1].Section
	if gotSections != "coordination,status" {
		t.Fatalf("sections = %q", gotSections)
	}
}

func TestHandleBlackboardExportJSONL(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"export", "coordination", "--jsonl"}, &stdout); err != nil {
		t.Fatalf("runBlackboard export jsonl: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	var lines int
	for scanner.Scan() {
		lines++
		var record blackboardExportRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode jsonl line %d: %v\n%s", lines, err, scanner.Text())
		}
		if record.Messaging.Text == "" {
			t.Fatalf("record missing messaging projection: %#v", record)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan jsonl: %v", err)
	}
	if lines != 1 {
		t.Fatalf("jsonl lines = %d, want 1", lines)
	}
}

func TestHandleBlackboardExportWorkflowMessagingJSON(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"export", "coordination", "--workflow-messaging", "--json"}, &stdout); err != nil {
		t.Fatalf("runBlackboard export workflow messaging: %v", err)
	}
	var records []blackboardExportRecord
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatalf("decode export json %q: %v", stdout.String(), err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	workflow := records[0].Workflow
	if workflow.StepType != "step.messaging_send" {
		t.Fatalf("workflow step type = %q", workflow.StepType)
	}
	if workflow.PluginFamily != "workflow-plugin-messaging-core" {
		t.Fatalf("workflow plugin family = %q", workflow.PluginFamily)
	}
	if workflow.Input.Text != "[coordination/status] ready" {
		t.Fatalf("workflow input text = %q", workflow.Input.Text)
	}
	if got := strings.Join(workflow.RequiredConfig, ","); got != "channel" {
		t.Fatalf("workflow required config = %q", got)
	}
}

func TestHandleBlackboardExportWorkflowMessagingJSONL(t *testing.T) {
	var stdout bytes.Buffer
	fake := &fakeBlackboardClient{}

	if err := runBlackboard(context.Background(), fake, []string{"export", "coordination", "--workflow-messaging", "--jsonl"}, &stdout); err != nil {
		t.Fatalf("runBlackboard export workflow messaging jsonl: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	var lines int
	for scanner.Scan() {
		lines++
		var record blackboardExportRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode jsonl line %d: %v\n%s", lines, err, scanner.Text())
		}
		if record.Workflow.StepType != "step.messaging_send" || record.Workflow.Input.Text == "" {
			t.Fatalf("record missing workflow projection: %#v", record)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan jsonl: %v", err)
	}
	if lines != 1 {
		t.Fatalf("jsonl lines = %d, want 1", lines)
	}
}

func TestHandleBlackboardExportRejectsCredentialFlags(t *testing.T) {
	for _, flag := range []string{"--webhook-url", "--channel", "--token", "--provider"} {
		err := runBlackboard(context.Background(), &fakeBlackboardClient{}, []string{"export", "coordination", flag, "secret"}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "unknown blackboard export flag") {
			t.Fatalf("runBlackboard export with %s error = %v, want unknown flag", flag, err)
		}
	}
}
