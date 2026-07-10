//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

type daemonLock struct {
	file *os.File
	once sync.Once
	err  error
}

func acquireDaemonLock() (*daemonLock, error) {
	file, err := os.OpenFile(filepath.Join(DataDir(), "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure daemon lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("daemon lock is held: %w", err)
	}
	return &daemonLock{file: file}, nil
}

func (l *daemonLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.err = errors.Join(
			unix.Flock(int(l.file.Fd()), unix.LOCK_UN),
			l.file.Close(),
		)
	})
	return l.err
}
