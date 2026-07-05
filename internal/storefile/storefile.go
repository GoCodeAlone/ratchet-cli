package storefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func WriteJSON(path string, value any, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create store temp file: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	cleanup := true
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write store temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod store temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync store temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close store temp file: %w", err)
	}
	closed = true
	if err := Replace(path, tmpPath); err != nil {
		return fmt.Errorf("replace store: %w", err)
	}
	cleanup = false
	return nil
}

func Replace(path, tmp string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	backupCreated := false
	if err := os.Rename(path, backup); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return err
		}
	} else {
		backupCreated = true
	}
	if err := os.Rename(tmp, path); err != nil {
		if backupCreated {
			_ = os.Rename(backup, path)
		}
		_ = os.Remove(tmp)
		return err
	}
	if backupCreated {
		_ = os.Remove(backup)
	}
	return nil
}
