package acpclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const archiveFormatVersion = 1

var (
	ErrSessionActive             = errors.New("acp client session is active")
	ErrSessionArchiveCollision   = errors.New("acp client archive destination session exists")
	ErrUnsupportedArchiveVersion = errors.New("unsupported acp client archive format version")
	ErrInvalidSessionArchive     = errors.New("invalid acp client archive")
)

type Archive struct {
	FormatVersion int                   `json:"format_version"`
	ExportedAt    string                `json:"exported_at"`
	ExportedBy    string                `json:"exported_by"`
	Session       ArchiveSession        `json:"session"`
	History       []ArchiveHistoryEvent `json:"history"`
}

type ArchiveSession struct {
	RecordID    string        `json:"record_id"`
	Name        *string       `json:"name,omitempty"`
	Agent       string        `json:"agent"`
	AgentName   string        `json:"agent_name,omitempty"`
	CWDRelative string        `json:"cwd_relative"`
	CWDOriginal string        `json:"cwd_original"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
	State       SessionRecord `json:"state"`
}

type ArchiveHistoryEvent struct {
	Kind       string    `json:"kind"`
	ID         string    `json:"id,omitempty"`
	Prompt     string    `json:"prompt,omitempty"`
	Response   string    `json:"response,omitempty"`
	Status     string    `json:"status,omitempty"`
	StopReason string    `json:"stop_reason,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
	UpdatedAt  time.Time `json:"updated_at,omitzero"`
}

type ExportOptions struct {
	HomeDir string
	Now     func() time.Time
}

type ImportOptions struct {
	SessionID          string
	Cwd                string
	HomeDir            string
	Agent              string
	CommandFingerprint string
}

func ExportSession(store *Store, id, outputPath string, opts ExportOptions) error {
	if store == nil {
		return errors.New("acp client store is required")
	}
	if id == "" {
		return errors.New("acp client session id is required")
	}
	if outputPath == "" {
		return errors.New("archive output path is required")
	}
	if _, err := store.Owner(id); err == nil {
		return fmt.Errorf("%w: %s", ErrSessionActive, id)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	now := archiveNow(opts.Now)
	cwdRelative := cwdRelativeToHome(rec.Cwd, archiveHomeDir(opts.HomeDir))
	archive := Archive{
		FormatVersion: archiveFormatVersion,
		ExportedAt:    now.Format(time.RFC3339Nano),
		ExportedBy:    "ratchet-cli",
		Session: ArchiveSession{
			RecordID:    rec.ID,
			Agent:       rec.Agent,
			CWDRelative: cwdRelative,
			CWDOriginal: cwdRelative,
			CreatedAt:   rec.CreatedAt.Format(time.RFC3339Nano),
			UpdatedAt:   rec.UpdatedAt.Format(time.RFC3339Nano),
			State:       rec,
		},
		History: archiveHistory(rec),
	}
	return writeJSONFileAtomic(outputPath, archive, 0o600)
}

func ImportSession(store *Store, archivePath string, opts ImportOptions) (SessionRecord, error) {
	if store == nil {
		return SessionRecord{}, errors.New("acp client store is required")
	}
	if archivePath == "" {
		return SessionRecord{}, errors.New("archive path is required")
	}
	var archive Archive
	if err := readJSONFile(archivePath, &archive); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionRecord{}, err
		}
		return SessionRecord{}, fmt.Errorf("%w: %v", ErrInvalidSessionArchive, err)
	}
	if archive.FormatVersion != archiveFormatVersion {
		return SessionRecord{}, fmt.Errorf("%w: %d", ErrUnsupportedArchiveVersion, archive.FormatVersion)
	}
	rec := archive.Session.State
	if rec.ID == "" {
		rec.ID = archive.Session.RecordID
	}
	if rec.ID == "" {
		return SessionRecord{}, fmt.Errorf("%w: missing session id", ErrInvalidSessionArchive)
	}
	if opts.SessionID != "" {
		rec.ID = opts.SessionID
	}
	if _, err := store.Get(rec.ID); err == nil {
		return SessionRecord{}, fmt.Errorf("%w: %s", ErrSessionArchiveCollision, rec.ID)
	} else if !errors.Is(err, ErrSessionNotFound) {
		return SessionRecord{}, err
	}
	if opts.Cwd != "" {
		cwd, err := normalizeImportCWD(opts.Cwd)
		if err != nil {
			return SessionRecord{}, err
		}
		rec.Cwd = cwd
	} else {
		cwd, err := resolveArchiveCWD(archive.Session.CWDRelative, archiveHomeDir(opts.HomeDir))
		if err != nil {
			return SessionRecord{}, err
		}
		rec.Cwd = cwd
	}
	if opts.Agent != "" {
		rec.Agent = opts.Agent
	}
	if opts.CommandFingerprint != "" {
		rec.CommandFingerprint = opts.CommandFingerprint
	}
	if rec.Status == SessionStatusRunning || rec.Status == SessionStatusCancelRequested || rec.Status == "" {
		rec.Status = SessionStatusCompleted
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = parseArchiveTime(archive.Session.CreatedAt)
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = parseArchiveTime(archive.Session.UpdatedAt)
	}
	if err := store.Upsert(rec); err != nil {
		return SessionRecord{}, err
	}
	return store.Get(rec.ID)
}

func archiveHistory(rec SessionRecord) []ArchiveHistoryEvent {
	history := make([]ArchiveHistoryEvent, 0, len(rec.Turns)+len(rec.PromptQueue))
	for _, turn := range rec.Turns {
		history = append(history, ArchiveHistoryEvent{
			Kind:       "turn",
			Prompt:     turn.Prompt,
			Response:   turn.Response,
			StopReason: turn.StopReason,
			CreatedAt:  turn.CreatedAt,
			UpdatedAt:  turn.CreatedAt,
		})
	}
	for _, item := range rec.PromptQueue {
		updated := item.CreatedAt
		if item.CompletedAt != nil {
			updated = *item.CompletedAt
		} else if item.CanceledAt != nil {
			updated = *item.CanceledAt
		} else if item.StartedAt != nil {
			updated = *item.StartedAt
		}
		history = append(history, ArchiveHistoryEvent{
			Kind:       "queue",
			ID:         item.ID,
			Prompt:     item.Prompt,
			Response:   item.Response,
			Status:     item.Status,
			StopReason: item.StopReason,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  updated,
		})
	}
	return history
}

func cwdRelativeToHome(cwd, home string) string {
	if cwd == "" {
		return "."
	}
	cwd = filepath.Clean(cwd)
	if home == "" {
		return cwd
	}
	home = filepath.Clean(home)
	rel, err := filepath.Rel(home, cwd)
	if err != nil || rel == "." {
		if err == nil {
			return "."
		}
		return cwd
	}
	if rel != ".." && !filepath.IsAbs(rel) && !startsWithParent(rel) {
		return rel
	}
	return cwd
}

func resolveArchiveCWD(cwdRelative, home string) (string, error) {
	if cwdRelative == "" || cwdRelative == "." {
		if home != "" {
			return filepath.Clean(home), nil
		}
		return ".", nil
	}
	cwdRelative = filepath.Clean(cwdRelative)
	if !filepath.IsAbs(cwdRelative) && startsWithParent(cwdRelative) {
		return "", fmt.Errorf("%w: cwd_relative escapes home: %s", ErrInvalidSessionArchive, cwdRelative)
	}
	if filepath.IsAbs(cwdRelative) || home == "" {
		return cwdRelative, nil
	}
	return filepath.Join(filepath.Clean(home), cwdRelative), nil
}

func normalizeImportCWD(cwd string) (string, error) {
	cwd = filepath.Clean(cwd)
	if filepath.IsAbs(cwd) {
		return cwd, nil
	}
	return filepath.Abs(cwd)
}

func startsWithParent(path string) bool {
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func archiveHomeDir(home string) string {
	if home != "" {
		return home
	}
	if envHome := os.Getenv("HOME"); envHome != "" {
		return envHome
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		return userHome
	}
	return ""
}

func archiveNow(clock func() time.Time) time.Time {
	if clock != nil {
		return clock().UTC()
	}
	return time.Now().UTC()
}

func parseArchiveTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
