//go:build unix

package acpclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type backgroundUnixAuditTransaction struct {
	path    string
	parent  *os.File
	file    *os.File
	release func() error
}

func backgroundWriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	if err := backgroundSyncDir(dir); err != nil {
		return newBackgroundPostCommitError(err)
	}
	return nil
}

func backgroundOpenPrivateAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	parent, err := backgroundOpenUnixPrivateDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	f, err := backgroundOpenUnixRegularAt(parent, path, unix.O_WRONLY|unix.O_APPEND, true)
	closeErr := parent.Close()
	if err != nil {
		return nil, errors.Join(err, closeErr)
	}
	if closeErr != nil {
		_ = f.Close()
		return nil, closeErr
	}
	return f, nil
}

func backgroundOpenAuditTransaction(path string, create bool) (backgroundAuditTransaction, error) {
	parentPath := filepath.Dir(path)
	parent, parentErr := backgroundOpenUnixPrivateDir(parentPath)
	if parentErr != nil && !errors.Is(parentErr, os.ErrNotExist) {
		return nil, parentErr
	}
	release, err := acquireStoreFileLock(path + ".lock")
	if err != nil {
		if parent != nil {
			_ = parent.Close()
		}
		return nil, err
	}
	if parent == nil {
		parent, err = backgroundOpenUnixPrivateDir(parentPath)
		if err != nil {
			return nil, errors.Join(err, release())
		}
	}
	f, err := backgroundOpenUnixRegularAt(parent, path, unix.O_RDWR, create)
	if errors.Is(err, os.ErrNotExist) && !create {
		return &backgroundUnixAuditTransaction{path: path, parent: parent, release: release}, nil
	}
	if err != nil {
		return nil, errors.Join(err, release(), parent.Close())
	}
	return &backgroundUnixAuditTransaction{path: path, parent: parent, file: f, release: release}, nil
}

func backgroundOpenUnixPrivateDir(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, storeLockUnsafePathError(path, err)
	}
	dir := os.NewFile(uintptr(fd), path)
	if dir == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create private directory handle")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = dir.Close()
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o777 != 0o700 {
		_ = dir.Close()
		return nil, storeLockUnsafePathError(path, fmt.Errorf("audit parent must be an owner-only directory (mode=%#o uid=%d)", stat.Mode&0o777, stat.Uid))
	}
	return dir, nil
}

func backgroundOpenUnixRegularAt(parent *os.File, path string, flags int, create bool) (*os.File, error) {
	name := filepath.Base(path)
	fd, err := unix.Openat(int(parent.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) && create {
		fd, err = unix.Openat(int(parent.Fd()), name, flags|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if errors.Is(err, unix.EEXIST) {
			fd, err = unix.Openat(int(parent.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
	}
	if err != nil {
		if errors.Is(err, unix.ENOENT) && !create {
			return nil, os.ErrNotExist
		}
		return nil, storeLockUnsafePathError(path, err)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create private file handle")
	}
	if err := backgroundValidateUnixRegular(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func backgroundValidateUnixRegular(f *os.File, path string) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o777 != 0o600 {
		return storeLockUnsafePathError(path, fmt.Errorf("target must be an owner-only single-link regular file (mode=%#o links=%d uid=%d)", stat.Mode&0o777, stat.Nlink, stat.Uid))
	}
	return nil
}

func (t *backgroundUnixAuditTransaction) File() *os.File { return t.file }

func (t *backgroundUnixAuditTransaction) ValidateForMutation() error {
	if t.file == nil {
		return storeLockUnsafePathError(t.path, os.ErrNotExist)
	}
	if err := backgroundValidateUnixRegular(t.file, t.path); err != nil {
		return err
	}
	check, err := backgroundOpenUnixRegularAt(t.parent, t.path, unix.O_RDONLY, false)
	if err != nil {
		return err
	}
	defer check.Close() //nolint:errcheck
	var current, reopened unix.Stat_t
	if err := unix.Fstat(int(t.file.Fd()), &current); err != nil {
		return err
	}
	if err := unix.Fstat(int(check.Fd()), &reopened); err != nil {
		return err
	}
	if current.Dev != reopened.Dev || current.Ino != reopened.Ino {
		return storeLockUnsafePathError(t.path, errors.New("audit target identity changed"))
	}
	return nil
}

func (t *backgroundUnixAuditTransaction) SyncParent() error { return t.parent.Sync() }

func (t *backgroundUnixAuditTransaction) Close() error {
	return errors.Join(t.release(), t.parent.Close())
}

func backgroundRemoveFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := backgroundSyncDir(filepath.Dir(path)); err != nil {
		return newBackgroundPostCommitError(err)
	}
	return nil
}

func backgroundSyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close() //nolint:errcheck
	return dir.Sync()
}

func backgroundSyncParentDir(path string) error {
	return backgroundSyncDir(path)
}
