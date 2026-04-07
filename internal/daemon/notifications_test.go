package daemon

import (
	"runtime"
	"testing"
)

func TestNotificationCommand(t *testing.T) {
	cmd := buildNotifyCommand("Team t-3a7f idle", "No activity for 5 minutes")
	if cmd == nil {
		t.Skip("no notification command available on this platform")
	}
	// Just verify the command structure, don't execute.
	switch runtime.GOOS {
	case "darwin":
		if cmd.Path == "" {
			t.Error("expected osascript path")
		}
	case "linux":
		if cmd.Path == "" {
			t.Error("expected notify-send path")
		}
	}
}
