package mesh

// ptyProviders is the set of provider names that use PTY sessions
// and should be routed through BBBridge instead of executor.Execute.
var ptyProviders = map[string]bool{
	"claude_code": true,
	"copilot_cli": true,
	"codex_cli":   true,
	"gemini_cli":  true,
	"cursor_cli":  true,
}

// IsPTYProvider returns true if the named provider requires a PTY session.
func IsPTYProvider(provider string) bool {
	return ptyProviders[provider]
}
