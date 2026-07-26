package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

var (
	exportReloadCheckpoint = ExportCheckpoint
	saveReloadCheckpoint   = SaveCheckpoint
)

const (
	daemonShutdownRPCTimeout = 2 * time.Second
	daemonGracePeriod        = 5 * time.Second
	daemonFallbackPeriod     = 5 * time.Second
	daemonPollInterval       = 100 * time.Millisecond
	daemonStopTimeout        = 15 * time.Second
)

var errNoDaemonRunning = errors.New("no daemon running")

type daemonLifecycleOps struct {
	readPID         func() (int, error)
	processRunning  func(int) bool
	shutdownRPC     func(context.Context) error
	terminate       func(int) error
	cleanupPID      func()
	cleanupSocket   func()
	startBackground func(bool) error
	pollInterval    time.Duration
	gracePeriod     time.Duration
	fallbackPeriod  time.Duration
}

type daemonReloadBarrier struct {
	mu         sync.Mutex
	closing    bool
	reloadDone chan struct{}
}

func (b *daemonReloadBarrier) beginReload() (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing || b.reloadDone != nil {
		return nil, false
	}
	b.reloadDone = make(chan struct{})
	return func() { close(b.reloadDone) }, true
}

func (b *daemonReloadBarrier) beginShutdown() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closing = true
	return b.reloadDone
}

// Start runs the daemon in the foreground. It creates the Unix socket,
// starts the gRPC server, and blocks until signal.
func Start(ctx context.Context, debug bool) error {
	if err := EnsureDataDir(); err != nil {
		return err
	}
	lock, err := acquireDaemonLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	reloadBarrier := &daemonReloadBarrier{}

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
	svc, err := NewDaemonService(runCtx)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() {
		reloadDone := reloadBarrier.beginShutdown()
		cancel()
		if reloadDone != nil {
			<-reloadDone
		}
		svc.Close()
	}()
	svc.engine.Debug = debug
	svc.SetShutdownFunc(cancel)
	pb.RegisterRatchetDaemonServer(srv, svc)

	// Graceful shutdown on the platform's configured daemon shutdown signals.
	signalCtx, stop := signal.NotifyContext(runCtx, shutdownSignals()...)
	defer stop()

	go func() {
		<-signalCtx.Done()
		reloadDone := reloadBarrier.beginShutdown()
		cancel()
		if reloadDone != nil {
			<-reloadDone
		}
		log.Println("shutting down daemon...")
		srv.GracefulStop()
	}()

	if reloadSignalsSupported() {
		// SIGUSR1 triggers a graceful reload: checkpoint state then stop so the
		// caller (CLI or new binary) can restart the daemon with the checkpoint.
		sigReload := make(chan os.Signal, 1)
		signal.Notify(sigReload, reloadSignal)
		defer signal.Stop(sigReload)
		go func() {
			select {
			case <-sigReload:
			case <-runCtx.Done():
				return
			}
			finishReload, ok := reloadBarrier.beginReload()
			if !ok {
				return
			}
			defer func() {
				finishReload()
				cancel()
			}()
			log.Println("reload signal received, checkpointing...")
			cp, err := exportReloadCheckpoint(svc)
			if err != nil {
				log.Printf("checkpoint failed: %v", err)
				return
			}
			if err := saveReloadCheckpoint(cp); err != nil {
				log.Printf("save checkpoint failed: %v", err)
			} else {
				log.Printf("checkpoint saved to %s", CheckpointPath())
			}
			log.Println("stopping daemon for reload...")
		}()
	}

	log.Printf("daemon listening on %s (pid %d)", SocketPath(), os.Getpid())
	return srv.Serve(lis)
}

// StartBackground forks the current process as a background daemon.
func StartBackground(debug bool) error {
	if IsRunning() {
		return nil // already running
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	args := []string{"daemon", "start"}
	if debug {
		args = append(args, "--debug")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = backgroundSysProcAttr()

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

func defaultDaemonLifecycleOps() daemonLifecycleOps {
	return daemonLifecycleOps{
		readPID:         ReadPID,
		processRunning:  processRunning,
		shutdownRPC:     requestDaemonShutdown,
		terminate:       terminateDaemon,
		cleanupPID:      CleanupPID,
		cleanupSocket:   CleanupSocket,
		startBackground: StartBackground,
		pollInterval:    daemonPollInterval,
		gracePeriod:     daemonGracePeriod,
		fallbackPeriod:  daemonFallbackPeriod,
	}
}

func requestDaemonShutdown(ctx context.Context) (retErr error) {
	conn, err := grpc.NewClient(
		"unix://"+SocketPath(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to daemon at %s: %w", SocketPath(), err)
	}
	defer func() {
		retErr = errors.Join(retErr, conn.Close())
	}()

	if _, err := pb.NewRatchetDaemonClient(conn).Shutdown(ctx, &pb.Empty{}); err != nil {
		return fmt.Errorf("request daemon shutdown: %w", err)
	}
	return nil
}

func terminateDaemon(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(terminateSignal()); err != nil {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	return nil
}

func waitForDaemonExit(
	ctx context.Context,
	pid int,
	limit time.Duration,
	ops daemonLifecycleOps,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	for ops.processRunning(pid) {
		select {
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-time.After(ops.pollInterval):
		}
	}
	return nil
}

func reconcileDaemonOwnership(oldPID int, ops daemonLifecycleOps) (bool, error) {
	currentPID, err := ops.readPID()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ops.cleanupPID()
			ops.cleanupSocket()
			return false, nil
		}
		return false, fmt.Errorf("read daemon PID ownership: %w", err)
	}

	if currentPID != oldPID && ops.processRunning(currentPID) {
		return true, nil
	}
	if currentPID == oldPID && ops.processRunning(currentPID) {
		return false, fmt.Errorf("daemon process %d is still running", currentPID)
	}

	ops.cleanupPID()
	ops.cleanupSocket()
	return false, nil
}

func stopContextWithOps(ctx context.Context, ops daemonLifecycleOps) (bool, error) {
	oldPID, err := ops.readPID()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ops.cleanupPID()
			ops.cleanupSocket()
			return false, errNoDaemonRunning
		}
		return false, fmt.Errorf("read daemon PID: %w", err)
	}
	if !ops.processRunning(oldPID) {
		return reconcileDaemonOwnership(oldPID, ops)
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("wait for daemon shutdown: %w", err)
	}

	rpcCtx, cancelRPC := context.WithTimeout(ctx, daemonShutdownRPCTimeout)
	rpcErr := ops.shutdownRPC(rpcCtx)
	cancelRPC()
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("wait for daemon shutdown: %w", err)
	}

	var gracefulErr error
	if rpcErr == nil {
		gracefulErr = waitForDaemonExit(ctx, oldPID, ops.gracePeriod, ops)
		if gracefulErr == nil {
			return reconcileDaemonOwnership(oldPID, ops)
		}
		if err := ctx.Err(); err != nil {
			return false, fmt.Errorf("wait for daemon shutdown: %w", err)
		}
	}

	if err := ops.terminate(oldPID); err != nil {
		terminateErr := fmt.Errorf("terminate daemon process %d: %w", oldPID, err)
		if rpcErr != nil {
			return false, errors.Join(rpcErr, terminateErr)
		}
		return false, errors.Join(
			fmt.Errorf("wait for graceful daemon shutdown: %w", gracefulErr),
			terminateErr,
		)
	}

	if err := waitForDaemonExit(ctx, oldPID, ops.fallbackPeriod, ops); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, fmt.Errorf("wait for daemon shutdown: %w", ctxErr)
		}
		fallbackErr := fmt.Errorf("wait for terminated daemon process %d: %w", oldPID, err)
		if rpcErr != nil {
			return false, errors.Join(rpcErr, fallbackErr)
		}
		return false, errors.Join(
			fmt.Errorf("wait for graceful daemon shutdown: %w", gracefulErr),
			fallbackErr,
		)
	}
	return reconcileDaemonOwnership(oldPID, ops)
}

// StopContext requests graceful shutdown and waits until the captured daemon
// process has exited or ctx is canceled.
func StopContext(ctx context.Context) error {
	_, err := stopContextWithOps(ctx, defaultDaemonLifecycleOps())
	return err
}

// Stop is the bounded compatibility wrapper for StopContext.
func Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), daemonStopTimeout)
	defer cancel()
	return StopContext(ctx)
}

func restartWithOps(ctx context.Context, debug bool, ops daemonLifecycleOps) error {
	replaced, err := stopContextWithOps(ctx, ops)
	if err != nil && !errors.Is(err, errNoDaemonRunning) {
		return err
	}
	if replaced {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("wait for daemon shutdown: %w", err)
	}
	if err := ops.startBackground(debug); err != nil {
		return fmt.Errorf("start replacement daemon: %w", err)
	}
	return nil
}

// Restart waits for the captured daemon process to exit before ensuring a
// replacement is ready.
func Restart(ctx context.Context, debug bool) error {
	return restartWithOps(ctx, debug, defaultDaemonLifecycleOps())
}

// Status returns daemon health info.
func Status() (string, error) {
	if !IsRunning() {
		return "daemon is not running", nil
	}
	pid, _ := ReadPID()
	return fmt.Sprintf("daemon running (pid %d, socket %s)", pid, SocketPath()), nil
}

// TriggerReload sends the platform's configured reload signal to the running
// daemon, causing it to checkpoint and exit gracefully. The caller is
// responsible for restarting. Platforms without reload signals return an error.
func TriggerReload() error {
	if !reloadSignalsSupported() {
		return fmt.Errorf("daemon reload signal is not supported on this platform")
	}
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
	cmd.SysProcAttr = backgroundSysProcAttr()
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
