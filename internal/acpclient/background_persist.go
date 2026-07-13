package acpclient

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
)

var backgroundPathLocks sync.Map

type backgroundPostCommitError struct {
	err error
}

func (e *backgroundPostCommitError) Error() string { return e.err.Error() }
func (e *backgroundPostCommitError) Unwrap() error { return e.err }

func newBackgroundPostCommitError(err error) error {
	if err == nil {
		return nil
	}
	return &backgroundPostCommitError{err: err}
}

func backgroundWriteCommitted(err error) bool {
	var committed *backgroundPostCommitError
	return errors.As(err, &committed)
}

func backgroundPostCommitCause(err error) error {
	var committed *backgroundPostCommitError
	if errors.As(err, &committed) {
		return committed.err
	}
	return err
}

func storeCommitUnconfirmed(errs ...error) error {
	return errors.Join(append([]error{ErrStoreCommitUnconfirmed}, errs...)...)
}

func backgroundPathLock(path string) *sync.Mutex {
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		cleaned = filepath.Clean(path)
	}
	lock, _ := backgroundPathLocks.LoadOrStore(cleaned, new(sync.Mutex))
	return lock.(*sync.Mutex)
}

func backgroundWriteJSONAtomic(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return backgroundWriteFileAtomic(path, b)
}
