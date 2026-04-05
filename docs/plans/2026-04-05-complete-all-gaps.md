# Complete All Remaining Gaps — Implementation Plan

**Goal:** Eliminate every stub, placeholder, and unimplemented feature in ratchet-cli so there are zero non-functional code paths.

**Architecture:** Implement 5 unimplemented RPCs (Shutdown, AttachSession, DetachSession, ListAgents, GetAgentStatus), add session broadcast fan-out, implement RemoteNode + mesh gRPC handlers, add model switching, fix teams nil-engine guard, clean up dead code, and write tests for everything.

**Tech Stack:** Go 1.26, gRPC, protobuf, Bubbletea v2, goakt v4

---

## Task 1: Add UpdateProviderModel proto + regenerate

**Files:**
- Modify: `internal/proto/ratchet.proto`
- Regenerate: `internal/proto/ratchet.pb.go`, `internal/proto/ratchet_grpc.pb.go`

**Step 1:** Add to `ratchet.proto` after `SetDefaultProviderReq` (~line 193):
```protobuf
message UpdateProviderModelReq {
  string alias = 1;
  string model = 2;
}
```

Add to the service block after `SetDefaultProvider`:
```protobuf
rpc UpdateProviderModel(UpdateProviderModelReq) returns (Empty);
```

**Step 2:** Regenerate:
```bash
cd /Users/jon/workspace/ratchet-cli
protoc --go_out=. --go-grpc_out=. internal/proto/ratchet.proto
```

**Step 3:** Build: `go build ./...`

**Step 4:** Commit: `proto: add UpdateProviderModel RPC`

---

## Task 2: Implement SessionBroadcaster for attach/detach support

**Files:**
- Create: `internal/daemon/session_broadcaster.go`
- Create: `internal/daemon/session_broadcaster_test.go`

**Step 1:** Create `SessionBroadcaster`:
```go
type SessionBroadcaster struct {
    mu   sync.RWMutex
    subs map[string]map[string]chan *pb.ChatEvent // sessionID → subID → channel
}

func NewSessionBroadcaster() *SessionBroadcaster
func (b *SessionBroadcaster) Subscribe(sessionID string) (ch <-chan *pb.ChatEvent, subID string)
func (b *SessionBroadcaster) Unsubscribe(sessionID, subID string)
func (b *SessionBroadcaster) Publish(sessionID string, event *pb.ChatEvent)
```

`Subscribe` creates a buffered channel (64), generates a uuid subID, stores it. `Publish` does non-blocking sends to all subscribers for that session. `Unsubscribe` removes and closes the channel.

**Step 2:** Write tests: TestSubscribePublish, TestMultipleSubscribers, TestUnsubscribe, TestPublishNoSubscribers.

**Step 3:** Build and test: `go test ./internal/daemon/ -run TestSessionBroadcaster -v`

**Step 4:** Commit: `feat: add SessionBroadcaster for multi-subscriber session events`

---

## Task 3: Implement Shutdown, AttachSession, DetachSession, ListAgents, GetAgentStatus, UpdateProviderModel RPCs

**Files:**
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/chat.go`
- Modify: `internal/daemon/teams.go`
- Modify: `internal/daemon/fleet.go`

**Step 1:** Add fields to `Service`:
```go
broadcaster  *SessionBroadcaster
shutdownFn   func() // set by daemon main, cancels root context
```

Initialize in `NewService`: `svc.broadcaster = NewSessionBroadcaster()`

Add `SetShutdownFunc(fn func())` method so daemon main can inject the cancel function.

**Step 2:** Implement `Shutdown`:
```go
func (s *Service) Shutdown(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
    if s.shutdownFn != nil {
        go func() {
            time.Sleep(100 * time.Millisecond) // let RPC response flush
            s.shutdownFn()
        }()
    }
    return &pb.Empty{}, nil
}
```

**Step 3:** Implement `AttachSession`:
```go
func (s *Service) AttachSession(req *pb.AttachReq, stream pb.RatchetDaemon_AttachSessionServer) error {
    ch, subID := s.broadcaster.Subscribe(req.SessionId)
    defer s.broadcaster.Unsubscribe(req.SessionId, subID)
    for {
        select {
        case event, ok := <-ch:
            if !ok { return nil }
            if err := stream.Send(event); err != nil { return err }
        case <-stream.Context().Done():
            return nil
        }
    }
}
```

**Step 4:** Implement `DetachSession`:
```go
func (s *Service) DetachSession(ctx context.Context, req *pb.DetachReq) (*pb.Empty, error) {
    // Detach is handled client-side by cancelling the AttachSession stream.
    // This RPC exists for explicit cleanup if needed.
    return &pb.Empty{}, nil
}
```

**Step 5:** Update `handleChat` in `chat.go` to publish events via broadcaster. After each `stream.Send(event)`, also call `s.broadcaster.Publish(sessionID, event)`.

**Step 6:** Add `ListAllAgents` to `TeamManager`:
```go
func (tm *TeamManager) ListAllAgents() []*pb.Agent {
    tm.mu.RLock()
    defer tm.mu.RUnlock()
    var agents []*pb.Agent
    for _, ti := range tm.teams {
        // ... collect agents from ti.agents
    }
    return agents
}
```

**Step 7:** Add `ListAllWorkers` to `FleetManager`:
```go
func (fm *FleetManager) ListAllWorkers() []*pb.Agent {
    fm.mu.RLock()
    defer fm.mu.RUnlock()
    var agents []*pb.Agent
    for _, fi := range fm.fleets {
        // ... map workers to pb.Agent
    }
    return agents
}
```

**Step 8:** Implement `ListAgents`:
```go
func (s *Service) ListAgents(ctx context.Context, _ *pb.Empty) (*pb.AgentList, error) {
    agents := s.teams.ListAllAgents()
    agents = append(agents, s.fleet.ListAllWorkers()...)
    return &pb.AgentList{Agents: agents}, nil
}
```

**Step 9:** Implement `GetAgentStatus` — search teams then fleets by agent ID.

**Step 10:** Implement `UpdateProviderModel`:
```go
func (s *Service) UpdateProviderModel(ctx context.Context, req *pb.UpdateProviderModelReq) (*pb.Empty, error) {
    _, err := s.engine.DB.ExecContext(ctx, "UPDATE llm_providers SET model = ? WHERE alias = ?", req.Model, req.Alias)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "update model: %v", err)
    }
    s.engine.ProviderRegistry.InvalidateCacheAlias(req.Alias)
    return &pb.Empty{}, nil
}
```

**Step 11:** Build: `go build ./...`

**Step 12:** Commit: `feat: implement Shutdown, Attach/Detach, ListAgents, GetAgentStatus, UpdateProviderModel RPCs`

---

## Task 4: Add client methods and wire TUI model switching

**Files:**
- Modify: `internal/client/client.go`
- Modify: `internal/tui/commands/commands.go`

**Step 1:** Add client methods:
```go
func (c *Client) Shutdown(ctx context.Context) error
func (c *Client) AttachSession(ctx context.Context, sessionID string) (<-chan *pb.ChatEvent, error)
func (c *Client) UpdateProviderModel(ctx context.Context, alias, model string) error
func (c *Client) ListAgents(ctx context.Context) (*pb.AgentList, error)
func (c *Client) GetAgentStatus(ctx context.Context, agentID string) (*pb.Agent, error)
```

**Step 2:** Wire `/model` command in `commands.go`. Replace the "not yet implemented" message with:
```go
// /model <alias> <model-name>
if err := c.UpdateProviderModel(context.Background(), args[0], args[1]); err != nil {
    return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
}
return &Result{Lines: []string{fmt.Sprintf("Updated %s model to %s", args[0], args[1])}}
```

**Step 3:** Build: `go build ./...`

**Step 4:** Commit: `feat: add client methods and wire TUI /model command`

---

## Task 5: Implement RemoteNode with gRPC mesh streaming

**Files:**
- Modify: `internal/mesh/remote_node.go`
- Modify: `internal/daemon/service.go` (RegisterMeshNode + MeshStream handlers)

**Step 1:** Implement `RemoteNode.Run`:
```go
func (n *RemoteNode) Run(ctx context.Context, task string, bb *Blackboard, inbox <-chan Message, outbox chan<- Message) error {
    conn, err := grpc.DialContext(ctx, n.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return fmt.Errorf("remote node %s: dial %s: %w", n.id, n.address, err)
    }
    defer conn.Close()

    client := pb.NewRatchetDaemonClient(conn)
    stream, err := client.MeshStream(ctx)
    if err != nil {
        return fmt.Errorf("remote node %s: open mesh stream: %w", n.id, n.address, err)
    }

    // Goroutine: forward outbox messages + blackboard writes to remote
    go func() { /* read outbox, convert to MeshEvent, stream.Send */ }()

    // Main loop: receive MeshEvents from remote, apply to local bb + outbox
    for { /* stream.Recv, apply BlackboardSync/AgentMessage */ }
}
```

**Step 2:** Implement `Service.RegisterMeshNode`:
```go
func (s *Service) RegisterMeshNode(ctx context.Context, req *pb.RegisterNodeReq) (*pb.RegisterNodeResp, error) {
    nodeID := uuid.New().String()
    // Register in the mesh (if a mesh team is active)
    return &pb.RegisterNodeResp{NodeId: nodeID}, nil
}
```

**Step 3:** Implement `Service.MeshStream` — bidirectional handler:
```go
func (s *Service) MeshStream(stream pb.RatchetDaemon_MeshStreamServer) error {
    // Read incoming MeshEvents, apply to local mesh blackboard/router
    // Forward local mesh events back to the remote node
}
```

**Step 4:** Update imports: add `grpc`, `insecure`, `pb` to remote_node.go

**Step 5:** Build: `go build ./...`

**Step 6:** Commit: `feat: implement RemoteNode gRPC execution and mesh stream handlers`

---

## Task 6: Fix teams nil-engine guard and clean up dead code

**Files:**
- Modify: `internal/daemon/teams.go`
- Modify: `internal/provider/auth.go`
- Modify: `internal/provider/oauth.go`

**Step 1:** Fix teams.go nil-engine guard — return error instead of fake success:
```go
if tm.engine == nil || tm.engine.ProviderRegistry == nil {
    return "", fmt.Errorf("no engine configured: cannot execute agent %s", ag.name)
}
```

**Step 2:** Fix `DeviceFlow()` in auth.go — wire to real implementation:
```go
func DeviceFlow(ctx context.Context) (string, error) {
    deviceResp, err := StartGitHubDeviceFlow(ctx, GithubCopilotClientID)
    if err != nil {
        return "", fmt.Errorf("start device flow: %w", err)
    }
    fmt.Printf("Open %s and enter code: %s\n", deviceResp.VerificationURI, deviceResp.UserCode)
    token, err := PollGitHubDeviceFlow(ctx, deviceResp, GithubCopilotClientID)
    if err != nil {
        return "", fmt.Errorf("poll device flow: %w", err)
    }
    return token, nil
}
```

**Step 3:** Fix `LocalOAuthServerPort()` in oauth.go — store port in an atomic variable when the server starts:
```go
var oauthServerPort atomic.Int32

// In startAnthropicOAuthFlow, after listener.Addr():
oauthServerPort.Store(int32(listener.Addr().(*net.TCPAddr).Port))

func LocalOAuthServerPort() int {
    return int(oauthServerPort.Load())
}
```

**Step 4:** Build and test: `go build ./...`

**Step 5:** Commit: `fix: teams nil-engine guard returns error, wire dead code stubs`

---

## Task 7: Write tests for all new functionality

**Files:**
- Create: `internal/daemon/shutdown_test.go`
- Create: `internal/daemon/attach_session_test.go`
- Create: `internal/daemon/list_agents_test.go`
- Create: `internal/daemon/model_switch_test.go`
- Create: `internal/mesh/remote_node_test.go` (update existing or create integration test)
- Create: `internal/daemon/mesh_stream_test.go`

**Step 1:** Shutdown test: verify shutdownFn is called.

**Step 2:** AttachSession test: subscribe, publish events, verify they arrive on the attach stream.

**Step 3:** ListAgents test: create teams + fleets with agents, verify aggregation.

**Step 4:** Model switch test: insert provider row, call UpdateProviderModel, verify DB updated + cache invalidated.

**Step 5:** RemoteNode test: in-process gRPC server, verify blackboard sync and message routing.

**Step 6:** MeshStream test: connect two in-process daemons, verify bidirectional event flow.

**Step 7:** Teams nil-engine test: verify executeAgent returns error when engine is nil.

**Step 8:** Run full suite: `go test -race ./... -count=1`

**Step 9:** Run linter: `golangci-lint run`

**Step 10:** Commit: `test: add tests for RPCs, remote mesh, model switching, nil-engine guard`

---

## Task 8: Final verification and cleanup

**Files:** Various — fix any remaining issues.

**Step 1:** Run `grep -rn "Unimplemented\|not yet implemented\|TODO\|FIXME\|stub\|placeholder" --include="*.go" | grep -v vendor | grep -v _test.go | grep -v docs/ | grep -v ".pb.go"` — verify ZERO results (except generated proto fallbacks).

**Step 2:** Run `go build ./...`

**Step 3:** Run `go test -race ./... -count=1`

**Step 4:** Run `golangci-lint run`

**Step 5:** Fix any issues found.

**Step 6:** Commit any fixes.

---

## Execution Order

```
Task 1 (proto) → Task 2 (broadcaster) → Task 3 (RPCs) → Task 4 (client + TUI)
                                                        → Task 5 (remote mesh)
                                                        → Task 6 (nil-engine + dead code)
                                       → Task 7 (tests) → Task 8 (verify)
```

Tasks 3-6 can run in parallel after Task 2. Task 7 depends on all. Task 8 is final verification.
