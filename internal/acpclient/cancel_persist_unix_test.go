//go:build unix

package acpclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancelRequestIsOwnerOnlyOnUnix(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 23, 15, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{ID: "private-cancel", Status: SessionStatusRunning, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.RequestCancel("private-cancel", now.Add(time.Second)); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	for _, path := range []string{filepath.Dir(store.cancelPath("private-cancel")), store.cancelPath("private-cancel")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat %s: %v", path, err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode %s = %o, want %o", path, got, want)
		}
	}
}
