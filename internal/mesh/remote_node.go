package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// RemoteNode is a mesh node that lives on a remote daemon and communicates
// over gRPC via the MeshStream RPC.
type RemoteNode struct {
	id      string
	info    NodeInfo
	address string
}

// NewRemoteNode creates a RemoteNode targeting the given gRPC address.
func NewRemoteNode(id, address string, info NodeInfo) *RemoteNode {
	return &RemoteNode{
		id:      id,
		info:    info,
		address: address,
	}
}

// ID returns the node's unique identifier.
func (n *RemoteNode) ID() string { return n.id }

// Info returns the node's metadata with Location set to the gRPC address.
func (n *RemoteNode) Info() NodeInfo {
	out := n.info
	out.Location = n.address
	return out
}

// Run dials the remote daemon, opens a bidirectional MeshStream, and bridges
// the local blackboard and message channels with the remote execution.
//
//   - Inbox messages (from other local nodes) are forwarded to the remote as AgentMessage events.
//   - Local blackboard writes are forwarded as BlackboardSync events.
//   - Incoming BlackboardSync events from the remote are applied to the local blackboard.
//   - Incoming AgentMessage events are routed to outbox for delivery to local nodes.
//
// Run returns when the context is cancelled, the remote signals completion via
// the blackboard ("status/<nodeID>" = "done" or "approved"), or the stream closes.
func (n *RemoteNode) Run(ctx context.Context, task string, bb *Blackboard, inbox <-chan Message, outbox chan<- Message) error {
	conn, err := grpc.NewClient(n.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("remote node %s: dial %s: %w", n.id, n.address, err)
	}
	defer conn.Close()

	client := pb.NewRatchetDaemonClient(conn)
	stream, err := client.MeshStream(ctx)
	if err != nil {
		return fmt.Errorf("remote node %s: open mesh stream at %s: %w", n.id, n.address, err)
	}

	// doneCh is closed when the remote signals task completion.
	doneCh := make(chan struct{})

	// Watch local blackboard for writes → forward to remote.
	bb.Watch(func(key string, val Entry) {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return
		}
		valueBytes, _ := json.Marshal(val.Value)
		_ = stream.Send(&pb.MeshEvent{
			Event: &pb.MeshEvent_BlackboardSync{
				BlackboardSync: &pb.BlackboardSync{
					Section:  parts[0],
					Key:      parts[1],
					Value:    valueBytes,
					Author:   val.Author,
					Revision: val.Revision,
				},
			},
		})
	})

	// Forward inbox messages to remote.
	go func() {
		for {
			select {
			case msg, ok := <-inbox:
				if !ok {
					return
				}
				_ = stream.Send(&pb.MeshEvent{
					Event: &pb.MeshEvent_AgentMessage{
						AgentMessage: &pb.AgentMessage{
							FromAgent: msg.From,
							ToAgent:   msg.To,
							Content:   msg.Content,
						},
					},
				})
			case <-ctx.Done():
				return
			case <-doneCh:
				return
			}
		}
	}()

	// Main loop: receive events from remote.
	for {
		ev, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("remote node %s: recv: %w", n.id, err)
		}
		switch e := ev.Event.(type) {
		case *pb.MeshEvent_BlackboardSync:
			sync := e.BlackboardSync
			var value any
			_ = json.Unmarshal(sync.Value, &value)
			bb.Write(sync.Section, sync.Key, value, sync.Author)
			// Check for task completion signal.
			if sync.Section == "status" && sync.Key == n.id {
				if s, ok := value.(string); ok && (s == "done" || s == "approved") {
					close(doneCh)
					return nil
				}
			}
		case *pb.MeshEvent_AgentMessage:
			msg := e.AgentMessage
			select {
			case outbox <- Message{
				ID:        fmt.Sprintf("%s-%d", n.id, time.Now().UnixNano()),
				From:      msg.FromAgent,
				To:        msg.ToAgent,
				Content:   msg.Content,
				Type:      "result",
				Timestamp: time.Now(),
			}:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
