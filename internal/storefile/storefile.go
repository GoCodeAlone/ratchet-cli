package storefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func WriteJSON(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write store temp file: %w", err)
	}
	if err := Replace(path, tmp); err != nil {
		return fmt.Errorf("replace store: %w", err)
	}
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
