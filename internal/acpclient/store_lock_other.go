//go:build !unix && !windows

package acpclient

func acquireStoreFileLock(path string) (func() error, error) {
	return nil, ErrStoreProcessLockUnsupported
}

func tryStoreFileLock(path string) (func() error, bool, error) {
	return nil, false, ErrStoreProcessLockUnsupported
}
