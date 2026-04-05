package mesh

// MeshProto defines the mesh-related message types that will be added to the
// protobuf schema in a future step. For now they serve as Go-native types.

// RegisterNodeReq is the request sent when a node joins the mesh.
type RegisterNodeReq struct {
	Name     string
	Role     string
	Model    string
	Provider string
	Tools    []string
}

// RegisterNodeResp is the response after a successful node registration.
type RegisterNodeResp struct {
	NodeID string
}

// BlackboardSync is a change notification for a single blackboard entry.
type BlackboardSync struct {
	Section  string
	Key      string
	Value    []byte
	Author   string
	Revision int64
}

// MeshEvent is a union type carrying one of several mesh-level events.
type MeshEvent struct {
	// One of:
	BlackboardSync *BlackboardSync
	AgentMessage   *Message
	NodeRegistered *RegisterNodeResp
}
