package daemon

import (
	"encoding/base64"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

// buildNotifyCommand returns an exec.Cmd for an OS-native notification.
// Returns nil if the platform is unsupported or the tool is not available.
func buildNotifyCommand(title, body string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		// Escape single quotes to prevent osascript injection.
		escapedBody := strings.ReplaceAll(body, `'`, `\'`)
		escapedTitle := strings.ReplaceAll(title, `'`, `\'`)
		script := `display notification "` + escapedBody + `" with title "` + escapedTitle + `"`
		return exec.Command("osascript", "-e", script)
	case "linux":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return nil
		}
		return exec.Command(path, title, body)
	case "windows":
		// Use -EncodedCommand to avoid PowerShell command injection.
		// Encode the script as UTF-16LE base64.
		script := fmt.Sprintf(`New-BurntToastNotification -Text '%s', '%s'`,
			strings.ReplaceAll(title, "'", "''"),
			strings.ReplaceAll(body, "'", "''"),
		)
		encoded := encodePS1(script)
		return exec.Command("powershell", "-EncodedCommand", encoded)
	default:
		return nil
	}
}

// encodePS1 encodes a PowerShell script as UTF-16LE base64 for -EncodedCommand.
func encodePS1(script string) string {
	// Encode as UTF-16LE (PowerShell -EncodedCommand expects this encoding).
	utf16 := make([]byte, 0, len(script)*2)
	for _, r := range script {
		utf16 = append(utf16, byte(r), byte(r>>8))
	}
	return base64.StdEncoding.EncodeToString(utf16)
}

// SendNotification sends an OS-native notification. Non-blocking; errors are logged.
func SendNotification(title, body string) {
	cmd := buildNotifyCommand(title, body)
	if cmd == nil {
		return
	}
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("notification: %v", err)
		}
	}()
}
