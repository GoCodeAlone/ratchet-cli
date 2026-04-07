package daemon

import (
	"testing"
)

func TestAutoresponder(t *testing.T) {
	rules := []AutorespondRule{
		{Match: "approval", Action: "approve"},
		{Match: "which.*approach", Action: "reply", Message: "Use the simpler approach."},
		{Match: "*", Action: "queue"},
	}
	ar := NewAutoresponder(rules)

	tests := []struct {
		question   string
		wantAction string
		wantMsg    string
	}{
		{"Can I get approval to deploy?", "approve", "approved"},
		{"Which approach should I take?", "reply", "Use the simpler approach."},
		{"Random question", "queue", ""},
	}

	for _, tt := range tests {
		action, msg := ar.Match(tt.question)
		if action != tt.wantAction {
			t.Errorf("Match(%q): action=%q, want %q", tt.question, action, tt.wantAction)
		}
		if tt.wantMsg != "" && msg != tt.wantMsg {
			t.Errorf("Match(%q): msg=%q, want %q", tt.question, msg, tt.wantMsg)
		}
	}
}
