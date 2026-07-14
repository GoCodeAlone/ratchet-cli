//go:build windows

package acpclient

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCancelRequestIsOwnerOnlyOnWindows(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 23, 15, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{ID: "private-cancel", Status: SessionStatusRunning, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.RequestCancel("private-cancel", now.Add(time.Second)); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(store.cancelPath("private-cancel")))
	assertBackgroundWindowsPrivateDACL(t, store.cancelPath("private-cancel"))
}
