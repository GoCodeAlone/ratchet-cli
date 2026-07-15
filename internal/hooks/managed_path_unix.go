//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package hooks

import (
	"errors"
	"io"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

var managedPolicyValidatePlatformACL = validateOpenedPlatformMutationACL

func defaultManagedPolicyPath() (string, error) {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/ratchet/managed-hooks.yaml", nil
	}
	return "/etc/ratchet/managed-hooks.yaml", nil
}

func secureReadManagedFile(path string) (data []byte, err error) {
	return secureReadManagedFileWithSnapshotReader(path, readManagedPolicySnapshot)
}

func secureReadManagedFileWithSnapshotReader(
	path string,
	readSnapshot managedPolicySnapshotReader,
) (data []byte, err error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create managed policy file handle")
	}
	defer finishManagedPolicyRead(&data, &err, file)

	initial, err := inspectManagedUnixSnapshot(file, fd)
	if err != nil {
		return nil, err
	}
	data, err = readManagedUnixSnapshotWith(file, initial, func() (managedUnixSnapshot, error) {
		return inspectManagedUnixSnapshot(file, fd)
	}, readSnapshot)
	return data, err
}

type managedUnixSnapshot struct {
	uid     uint32
	mode    uint32
	size    uint64
	modTime time.Time
}

func inspectManagedUnixSnapshot(file *os.File, fd int) (managedUnixSnapshot, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return managedUnixSnapshot{}, err
	}
	if stat.Size < 0 {
		return managedUnixSnapshot{}, errManagedPolicyChanged
	}
	if err := validateManagedPolicySize(uint64(stat.Size)); err != nil {
		return managedUnixSnapshot{}, err
	}
	if err := managedPolicyValidatePlatformACL(file.Name(), fd); err != nil {
		return managedUnixSnapshot{}, err
	}
	if err := validateManagedUnixMetadata(stat.Uid, uint32(stat.Mode)); err != nil {
		return managedUnixSnapshot{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return managedUnixSnapshot{}, err
	}
	if info.Size() != stat.Size {
		return managedUnixSnapshot{}, errManagedPolicyChanged
	}
	return managedUnixSnapshot{
		uid:     stat.Uid,
		mode:    uint32(stat.Mode),
		size:    uint64(stat.Size),
		modTime: info.ModTime(),
	}, nil
}

func readManagedUnixSnapshot(
	reader io.Reader,
	initial managedUnixSnapshot,
	inspect func() (managedUnixSnapshot, error),
) ([]byte, error) {
	return readManagedUnixSnapshotWith(reader, initial, inspect, readManagedPolicySnapshot)
}

func readManagedUnixSnapshotWith(
	reader io.Reader,
	initial managedUnixSnapshot,
	inspect func() (managedUnixSnapshot, error),
	readSnapshot managedPolicySnapshotReader,
) ([]byte, error) {
	return readSnapshot(reader, initial.size, func() error {
		current, err := inspect()
		if err != nil {
			return err
		}
		if current != initial {
			return errManagedPolicyChanged
		}
		return nil
	})
}

func validateManagedUnixMetadata(uid, mode uint32) error {
	if mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("managed policy is not a regular file")
	}
	if uid != 0 {
		return errors.New("managed policy is not owned by root")
	}
	if mode&0o022 != 0 {
		return errors.New("managed policy is group or other writable")
	}
	return nil
}
