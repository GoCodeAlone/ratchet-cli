//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

func acquireStoreFileLock(path string) (func() error, error) {
	release, acquired, err := lockStoreFile(path, false)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("blocking store lock was not acquired")
	}
	return release, nil
}

func tryStoreFileLock(path string) (func() error, bool, error) {
	return lockStoreFile(path, true)
}

func lockStoreFile(path string, nonblocking bool) (func() error, bool, error) {
	physicalPath, err := storeLockPhysicalPath(path)
	if err != nil {
		return nil, false, err
	}
	dir, err := openStoreLockDirectory(filepath.Dir(physicalPath))
	if err != nil {
		return nil, false, err
	}
	fd, err := openStoreLockFileAt(int(dir.Fd()), filepath.Base(physicalPath))
	closeDirErr := dir.Close()
	if err != nil {
		return nil, false, storeLockUnsafePathError(physicalPath, err)
	}
	if closeDirErr != nil {
		_ = unix.Close(fd)
		return nil, false, closeDirErr
	}
	f := os.NewFile(uintptr(fd), physicalPath)
	if f == nil {
		_ = unix.Close(fd)
		return nil, false, errors.New("create store lock file handle")
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	if err := unix.Flock(int(f.Fd()), flags); err != nil {
		_ = f.Close()
		if nonblocking && (errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(f.Fd()), unix.LOCK_UN), f.Close())
	}, true, nil
}

func openStoreLockFileAt(dirFD int, name string) (int, error) {
	var err error
	for range 16 {
		var fd int
		fd, err = unix.Openat(dirFD, name, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if !errors.Is(err, unix.ENOENT) {
			return fd, err
		}
		runtime.Gosched()
	}
	return -1, err
}

func openStoreLockDirectory(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, storeLockUnsafePathError(path, err)
	}
	dir := os.NewFile(uintptr(fd), path)
	if dir == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create store lock directory handle")
	}
	if err := dir.Chmod(0o700); err != nil {
		_ = dir.Close()
		return nil, err
	}
	return dir, nil
}
