//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

type daemonLock struct {
	file       *os.File
	overlapped windows.Overlapped
	once       sync.Once
	err        error
}

func acquireDaemonLock() (*daemonLock, error) {
	file, err := os.OpenFile(filepath.Join(DataDir(), "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}
	lock := &daemonLock{file: file}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&lock.overlapped,
	)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("daemon lock is held: %w", err)
	}
	return lock, nil
}

func (l *daemonLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.err = errors.Join(
			windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped),
			l.file.Close(),
		)
	})
	return l.err
}
