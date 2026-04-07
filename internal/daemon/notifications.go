package daemon

import (
	"log"
	"os/exec"
	"runtime"
)

// buildNotifyCommand returns an exec.Cmd for an OS-native notification.
// Returns nil if the platform is unsupported or the tool is not available.
func buildNotifyCommand(title, body string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + body + `" with title "` + title + `"`
		return exec.Command("osascript", "-e", script)
	case "linux":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return nil
		}
		return exec.Command(path, title, body)
	case "windows":
		return exec.Command("powershell", "-Command",
			`New-BurntToastNotification -Text "`+title+`", "`+body+`"`)
	default:
		return nil
	}
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
