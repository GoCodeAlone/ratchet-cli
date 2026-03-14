// Hand-written extension to ratchet proto for version check and reload RPCs.
// These types use standard protobuf struct tags for wire encoding without
// requiring regeneration of the full ratchet.proto descriptor.

package proto

// VersionCheckReq is sent by the CLI on connect to verify compatibility.
type VersionCheckReq struct {
	CliVersion      string `protobuf:"bytes,1,opt,name=cli_version,json=cliVersion,proto3" json:"cli_version,omitempty"`
	CliCommit       string `protobuf:"bytes,2,opt,name=cli_commit,json=cliCommit,proto3" json:"cli_commit,omitempty"`
	CliProtoVersion int32  `protobuf:"varint,3,opt,name=cli_proto_version,json=cliProtoVersion,proto3" json:"cli_proto_version,omitempty"`
}

func (x *VersionCheckReq) Reset()         { *x = VersionCheckReq{} }
func (x *VersionCheckReq) String() string  { return x.CliVersion }
func (x *VersionCheckReq) ProtoMessage()  {}

func (x *VersionCheckReq) GetCliVersion() string {
	if x != nil {
		return x.CliVersion
	}
	return ""
}

func (x *VersionCheckReq) GetCliCommit() string {
	if x != nil {
		return x.CliCommit
	}
	return ""
}

func (x *VersionCheckReq) GetCliProtoVersion() int32 {
	if x != nil {
		return x.CliProtoVersion
	}
	return 0
}

// VersionCheckResp reports compatibility and whether a reload is recommended.
type VersionCheckResp struct {
	Compatible        bool   `protobuf:"varint,1,opt,name=compatible,proto3" json:"compatible,omitempty"`
	ReloadRecommended bool   `protobuf:"varint,2,opt,name=reload_recommended,json=reloadRecommended,proto3" json:"reload_recommended,omitempty"`
	DaemonVersion     string `protobuf:"bytes,3,opt,name=daemon_version,json=daemonVersion,proto3" json:"daemon_version,omitempty"`
	Message           string `protobuf:"bytes,4,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *VersionCheckResp) Reset()         { *x = VersionCheckResp{} }
func (x *VersionCheckResp) String() string  { return x.DaemonVersion }
func (x *VersionCheckResp) ProtoMessage()  {}

func (x *VersionCheckResp) GetCompatible() bool {
	if x != nil {
		return x.Compatible
	}
	return false
}

func (x *VersionCheckResp) GetReloadRecommended() bool {
	if x != nil {
		return x.ReloadRecommended
	}
	return false
}

func (x *VersionCheckResp) GetDaemonVersion() string {
	if x != nil {
		return x.DaemonVersion
	}
	return ""
}

func (x *VersionCheckResp) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

// ReloadReq requests the daemon to checkpoint and restart using a new binary.
type ReloadReq struct {
	NewBinaryPath string `protobuf:"bytes,1,opt,name=new_binary_path,json=newBinaryPath,proto3" json:"new_binary_path,omitempty"`
	CliVersion    string `protobuf:"bytes,2,opt,name=cli_version,json=cliVersion,proto3" json:"cli_version,omitempty"`
}

func (x *ReloadReq) Reset()         { *x = ReloadReq{} }
func (x *ReloadReq) String() string  { return x.NewBinaryPath }
func (x *ReloadReq) ProtoMessage()  {}

func (x *ReloadReq) GetNewBinaryPath() string {
	if x != nil {
		return x.NewBinaryPath
	}
	return ""
}

func (x *ReloadReq) GetCliVersion() string {
	if x != nil {
		return x.CliVersion
	}
	return ""
}

// ReloadStatus streams progress events during a daemon reload.
type ReloadStatus struct {
	Status  string `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	Message string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *ReloadStatus) Reset()         { *x = ReloadStatus{} }
func (x *ReloadStatus) String() string  { return x.Status }
func (x *ReloadStatus) ProtoMessage()  {}

func (x *ReloadStatus) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *ReloadStatus) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}
