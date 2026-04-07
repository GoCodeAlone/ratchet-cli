package mesh

import (
	"testing"
)

func TestIsPTYProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"claude_code", true},
		{"copilot_cli", true},
		{"ollama", false},
		{"anthropic", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsPTYProvider(tt.provider); got != tt.want {
			t.Errorf("IsPTYProvider(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}
