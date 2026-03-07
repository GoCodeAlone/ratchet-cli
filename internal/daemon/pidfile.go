package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet")
}

func SocketPath() string {
	return filepath.Join(DataDir(), "daemon.sock")
}

func PIDPath() string {
	return filepath.Join(DataDir(), "daemon.pid")
}

func DBPath() string {
	return filepath.Join(DataDir(), "ratchet.db")
}

func EnsureDataDir() error {
	dirs := []string{
		DataDir(),
		filepath.Join(DataDir(), "plugins"),
		filepath.Join(DataDir(), "skills"),
		filepath.Join(DataDir(), "agents"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	return nil
}

func WritePID() error {
	return os.WriteFile(PIDPath(), []byte(strconv.Itoa(os.Getpid())), 0600)
}

func ReadPID() (int, error) {
	data, err := os.ReadFile(PIDPath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func CleanupPID() {
	os.Remove(PIDPath())
}

func CleanupSocket() {
	os.Remove(SocketPath())
}

// IsRunning checks if a daemon process is alive.
func IsRunning() bool {
	pid, err := ReadPID()
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without sending a signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
