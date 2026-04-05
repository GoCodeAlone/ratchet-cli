# Complete All Remaining Gaps — Design

**Date:** 2026-04-05
**Repo:** ratchet-cli
**Goal:** Eliminate every stub, placeholder, and unimplemented feature so ratchet-cli has zero non-functional code paths.

## 1. Shutdown RPC

`Service.Shutdown` cancels the daemon's root context, writes a checkpoint, then exits. The `Service` struct gains a `shutdownFn func()` field set by the daemon's `main()` via `NewService`. The RPC calls `shutdownFn()` which triggers `context.Cancel` → graceful teardown → `os.Exit(0)`.

## 2. AttachSession / DetachSession

Add a `SessionBroadcaster` per session that fans out `ChatEvent`s to multiple subscribers. When `handleChat` sends events, it writes to the broadcaster instead of directly to the gRPC stream. The original `SendMessage` stream is one subscriber.

- `AttachSession(req)` → subscribes to the broadcaster for `req.SessionId`, streams events to the caller
- `DetachSession(req)` → unsubscribes from the broadcaster

`SessionBroadcaster` struct:
- `Subscribe(sessionID) (<-chan *pb.ChatEvent, string)` — returns event channel + subscription ID
- `Unsubscribe(sessionID, subID)`
- `Publish(sessionID, event)` — non-blocking fan-out to all subscribers

## 3. ListAgents / GetAgentStatus

`TeamManager` and `FleetManager` already track agents internally. Add public methods:
- `TeamManager.ListAllAgents() []*pb.Agent` — iterates all active teams, collects agents
- `FleetManager.ListAllWorkers() []*pb.Agent` — iterates all active fleets, maps workers to Agent proto

`Service.ListAgents` aggregates from both managers. `Service.GetAgentStatus` looks up by agent ID across both.

## 4. RemoteNode + Mesh gRPC RPCs

### RemoteNode.Run
1. Dial the remote daemon at `n.address` via `grpc.Dial`
2. Call `MeshStream` to open bidirectional stream
3. Goroutine 1: read outbox channel → send `MeshEvent` (BlackboardSync or AgentMessage) over stream
4. Goroutine 2: read stream → apply incoming BlackboardSync to local blackboard, route AgentMessages via outbox
5. Main goroutine: block until context cancelled or stream closed
6. Return result based on blackboard `status/<nodeID>` value

### Service.RegisterMeshNode
Creates a `RemoteNode` in the daemon's `AgentMesh` registry. Returns the generated node ID. The remote daemon calls this when it wants to join a mesh team.

### Service.MeshStream
Bidirectional handler:
- Incoming `BlackboardSync` → write to the local mesh blackboard
- Incoming `AgentMessage` → route via the local mesh router
- Outgoing: watch the local blackboard and router for events relevant to the remote node, send them back

## 5. Model Switching (/model command)

Add `UpdateProviderModel` RPC:
```protobuf
message UpdateProviderModelReq {
  string alias = 1;
  string model = 2;
}
rpc UpdateProviderModel(UpdateProviderModelReq) returns (Empty);
```

Implementation: `UPDATE llm_providers SET model = ? WHERE alias = ?` + `InvalidateCacheAlias(alias)`.

TUI `/model <alias> <model>` command calls this RPC. Success message shows the updated provider.

## 6. Teams.go Nil-Engine Guard

Replace the fake success with an error:
```go
if tm.engine == nil || tm.engine.ProviderRegistry == nil {
    return "", fmt.Errorf("no engine configured: cannot execute agent %s", ag.name)
}
```

Tests that need nil-engine behavior explicitly pass a `mockEngine` with a mock provider instead of relying on the nil guard.

## 7. Dead Code Cleanup

- `auth.go:DeviceFlow()` → wire to `StartGitHubDeviceFlow` + `PollGitHubDeviceFlow`, add `context.Context` param
- `oauth.go:LocalOAuthServerPort()` → `startAnthropicOAuthFlow` stores port in a package-level `atomic.Int32`; `LocalOAuthServerPort()` reads it
- Remove any other dead/unreachable functions discovered during implementation

## 8. Testing Strategy

| Test | Type | What it validates |
|---|---|---|
| `shutdown_test.go` | Unit | Shutdown RPC calls cancel func |
| `session_broadcaster_test.go` | Unit | Subscribe/publish/unsubscribe, fan-out to multiple subscribers |
| `attach_session_test.go` | Integration | AttachSession receives events published by SendMessage |
| `list_agents_test.go` | Unit | Aggregates agents from teams + fleets |
| `remote_node_test.go` | Integration | RemoteNode.Run with in-process gRPC server |
| `mesh_stream_test.go` | Integration | MeshStream blackboard sync + message routing |
| `model_switch_test.go` | Unit | UpdateProviderModel updates DB + invalidates cache |
| `teams_nil_engine_test.go` | Unit | executeAgent returns error when engine is nil |

## Files Changed

| File | Change |
|---|---|
| `internal/daemon/service.go` | Implement Shutdown, AttachSession, DetachSession, ListAgents, GetAgentStatus, UpdateProviderModel |
| `internal/daemon/session_broadcaster.go` | NEW |
| `internal/daemon/chat.go` | Publish events via broadcaster |
| `internal/daemon/teams.go` | Fix nil-engine guard, add ListAllAgents |
| `internal/daemon/fleet.go` | Add ListAllWorkers |
| `internal/mesh/remote_node.go` | Real gRPC execution |
| `internal/proto/ratchet.proto` | Add UpdateProviderModelReq + RPC |
| `internal/proto/ratchet.pb.go` | Regenerated |
| `internal/proto/ratchet_grpc.pb.go` | Regenerated |
| `internal/tui/commands/commands.go` | Wire /model to RPC |
| `internal/client/client.go` | Add new RPC client methods |
| `internal/provider/auth.go` | Wire DeviceFlow |
| `internal/provider/oauth.go` | Surface OAuth port |
| Tests | ~8 new test files |
