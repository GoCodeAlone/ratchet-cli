package acpclient

import (
	"encoding/json"
	"path/filepath"
	"sync"
)

var backgroundPathLocks sync.Map

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
