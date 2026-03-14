package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// reloadSignal is the signal used to trigger graceful reload. Exported as a
// variable so tests can override it on platforms where SIGUSR1 is unavailable.
var reloadSignal = syscall.SIGUSR1

// Start runs the daemon in the foreground. It creates the Unix socket,
// starts the gRPC server, and blocks until signal.
func Start(ctx context.Context) error {
	if err := EnsureDataDir(); err != nil {
		return err
	}

	if IsRunning() {
		return fmt.Errorf("daemon already running (pid file: %s)", PIDPath())
	}

	CleanupSocket()

	if err := WritePID(); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer CleanupPID()

	lis, err := net.Listen("unix", SocketPath())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", SocketPath(), err)
	}
	defer lis.Close()
	defer CleanupSocket()

	// Set socket permissions
	if err := os.Chmod(SocketPath(), 0600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	srv := grpc.NewServer()
	svc, err := NewService(ctx)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	pb.RegisterRatchetDaemonServer(srv, svc)

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down daemon...")
		srv.GracefulStop()
	}()

	// SIGUSR1 triggers a graceful reload: checkpoint state then stop so the
	// caller (CLI or new binary) can restart the daemon with the checkpoint.
	sigReload := make(chan os.Signal, 1)
	signal.Notify(sigReload, reloadSignal)
	go func() {
		<-sigReload
		log.Println("reload signal received, checkpointing...")
		cp, err := ExportCheckpoint(svc)
		if err != nil {
			log.Printf("checkpoint failed: %v", err)
			srv.GracefulStop()
			return
		}
		if err := SaveCheckpoint(cp); err != nil {
			log.Printf("save checkpoint failed: %v", err)
		} else {
			log.Printf("checkpoint saved to %s", CheckpointPath())
		}
		log.Println("stopping daemon for reload...")
		srv.GracefulStop()
	}()

	log.Printf("daemon listening on %s (pid %d)", SocketPath(), os.Getpid())
	return srv.Serve(lis)
}

// StartBackground forks the current process as a background daemon.
func StartBackground() error {
	if IsRunning() {
		return nil // already running
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Wait for socket to appear
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(SocketPath()); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within 5s")
}

// Stop sends SIGTERM to the running daemon.
func Stop() error {
	pid, err := ReadPID()
	if err != nil {
		return fmt.Errorf("no daemon running")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return proc.Signal(syscall.SIGTERM)
}

// Status returns daemon health info.
func Status() (string, error) {
	if !IsRunning() {
		return "daemon is not running", nil
	}
	pid, _ := ReadPID()
	return fmt.Sprintf("daemon running (pid %d, socket %s)", pid, SocketPath()), nil
}

// TriggerReload sends SIGUSR1 to the running daemon, which causes it to
// checkpoint and exit gracefully. The caller is responsible for restarting.
func TriggerReload() error {
	pid, err := ReadPID()
	if err != nil {
		return fmt.Errorf("no daemon running")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return proc.Signal(reloadSignal)
}

// ReloadDaemon performs a full graceful reload:
//  1. Sends SIGUSR1 to the daemon (checkpoint + graceful stop).
//  2. Waits for the old process to exit.
//  3. Starts the new binary as a background daemon.
//
// newBinaryPath should be the path to the updated binary (typically
// os.Executable() from the new CLI process).
func ReloadDaemon(newBinaryPath string) error {
	if newBinaryPath == "" {
		var err error
		newBinaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("get executable: %w", err)
		}
	}

	// 1. Signal the running daemon to checkpoint and stop.
	if err := TriggerReload(); err != nil {
		return fmt.Errorf("trigger reload: %w", err)
	}

	// 2. Wait for the old daemon to finish (socket disappears).
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(SocketPath()); os.IsNotExist(err) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 3. Start the new daemon.
	cmd := exec.Command(newBinaryPath, "daemon", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new daemon: %w", err)
	}

	// Wait for new socket to appear.
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(SocketPath()); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("new daemon did not start within 5s")
}
