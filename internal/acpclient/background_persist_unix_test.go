//go:build unix

package acpclient

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundWriteFileAtomicReplacesOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "background.json")
	for _, data := range [][]byte{[]byte("first\n"), []byte("second\n")} {
		if err := backgroundWriteFileAtomic(path, data); err != nil {
			t.Fatalf("backgroundWriteFileAtomic: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != string(data) {
			t.Fatalf("content = %q, want %q", got, data)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("mode = %o, want 600", got)
		}
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".background.json.*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %#v", temps)
	}
}

// The audit namespace is the trust boundary: callers provide an owner-only
// parent, and mutation never relaxes its permissions or follows final links.
func TestBackgroundAuditUnixRequiresOwnerOnlyParent(t *testing.T) {
	dir := t.TempDir()
	audit := NewBackgroundAudit(filepath.Join(dir, "audit.jsonl"))
	namespace := filepath.Dir(audit.Path())
	if err := os.Mkdir(namespace, 0o755); err != nil {
		t.Fatalf("Mkdir weak namespace: %v", err)
	}
	err := audit.Append(backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	if !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	if _, err := os.Lstat(audit.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audit target exists after rejected parent: %v", err)
	}
	info, err := os.Stat(namespace)
	if err != nil {
		t.Fatalf("Stat parent: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("namespace mode = %o, want unchanged 755", got)
	}
}

func TestBackgroundAuditUnixRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("unchanged\n"), 0o600); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	audit := NewBackgroundAudit(filepath.Join(dir, "audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll namespace: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	err := audit.Append(backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	if !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(raw) != "unchanged\n" {
		t.Fatalf("symlink target mutated: %q", raw)
	}
}

func TestBackgroundAuditUnixRejectsHardLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("unchanged\n"), 0o600); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	audit := NewBackgroundAudit(filepath.Join(dir, "audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll namespace: %v", err)
	}
	if err := os.Link(target, path); err != nil {
		t.Fatalf("Link: %v", err)
	}
	err := audit.Append(backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	if !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(raw) != "unchanged\n" {
		t.Fatalf("hard-link target mutated: %q", raw)
	}
}

func TestBackgroundAuditUnixRejectsNonRegularTarget(t *testing.T) {
	dir := t.TempDir()
	audit := NewBackgroundAudit(filepath.Join(dir, "audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll namespace: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir target: %v", err)
	}
	err := audit.Append(backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	if !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
}

func TestBackgroundAuditUnixRejectsPostOpenReplacementBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	audit := NewBackgroundAudit(filepath.Join(dir, "audit.jsonl"))
	first := backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted)
	if err := audit.Append(first); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("unchanged\n"), 0o600); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	original := audit.Path() + ".original"
	audit.beforeMutation = func() {
		if err := os.Rename(audit.Path(), original); err != nil {
			t.Fatalf("Rename audit: %v", err)
		}
		if err := os.Symlink(target, audit.Path()); err != nil {
			t.Fatalf("Symlink replacement: %v", err)
		}
	}
	err := audit.Append(backgroundAuditTestRecord("event-2", BackgroundAuditStop, BackgroundOutcomeStopped))
	if !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(raw) != "unchanged\n" {
		t.Fatalf("replacement target mutated: %q", raw)
	}
	records, err := NewBackgroundAudit(original).Read()
	if err != nil {
		t.Fatalf("Read original: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != first.RecordID {
		t.Fatalf("original records = %#v, want unmodified first record", records)
	}
}
