package mesh

import "context"

// Node is the interface every mesh participant implements.
type Node interface {
	// ID returns the unique identifier for this node.
	ID() string

	// Run executes the node's main loop. It reads from inbox, writes to
	// outbox, and uses bb for shared state. The method blocks until the
	// task is complete, the context is cancelled, or a stop condition is met.
	Run(ctx context.Context, task string, bb *Blackboard, inbox <-chan Message, outbox chan<- Message) error

	// Info returns static metadata about this node.
	Info() NodeInfo
}

// NodeInfo describes a mesh node's identity and capabilities.
type NodeInfo struct {
	Name     string
	Role     string
	Model    string
	Provider string
	Location string // "local" or "grpc://host:port"
}

// NodeConfig holds everything needed to construct a LocalNode.
type NodeConfig struct {
	Name          string
	Role          string
	Model         string
	Provider      string
	Location      string // "local" or "grpc://host:port"
	SystemPrompt  string
	Tools         []string
	MaxIterations int
}
