package mesh

import (
	"context"
	"errors"
	"log"
)

// ErrNotImplemented is returned by RemoteNode operations that are stubbed out.
var ErrNotImplemented = errors.New("remote node execution not yet implemented")

// RemoteNode is a placeholder for a mesh node that lives on a remote host and
// communicates over gRPC. The implementation will be completed in a future step.
type RemoteNode struct {
	id      string
	info    NodeInfo
	address string
}

// NewRemoteNode creates a stub RemoteNode targeting the given gRPC address.
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

// Run is not yet implemented for remote nodes.
func (n *RemoteNode) Run(_ context.Context, _ string, _ *Blackboard, _ <-chan Message, _ chan<- Message) error {
	log.Printf("remote node %s (%s): execution not yet implemented", n.id, n.address)
	return ErrNotImplemented
}
