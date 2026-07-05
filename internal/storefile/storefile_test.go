package storefile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONCreatesParentDirectoryAndRewritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "store.json")

	if err := WriteJSON(path, map[string]string{"status": "first"}, 0o600); err != nil {
		t.Fatalf("WriteJSON first: %v", err)
	}
	if err := WriteJSON(path, map[string]string{"status": "second"}, 0o600); err != nil {
		t.Fatalf("WriteJSON second: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte(`"status": "second"`)) {
		t.Fatalf("store did not contain rewritten value: %s", data)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup file still exists or unexpected stat error: %v", err)
	}
}
