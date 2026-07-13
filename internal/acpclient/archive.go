package acpclient

import (
	"encoding/json"
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
	FormatVersion  int                   `json:"format_version"`
	ExportedAt     string                `json:"exported_at"`
	ExportedBy     string                `json:"exported_by"`
	Session        ArchiveSession        `json:"session"`
	History        []ArchiveHistoryEvent `json:"history,omitempty"`
	SummaryHistory []ArchiveHistoryEvent `json:"summary_history,omitempty"`
	RawHistory     []json.RawMessage     `json:"-"`
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
	HomeDir     string
	Now         func() time.Time
	HistoryMode ArchiveHistoryMode
}

type ArchiveHistoryMode string

const (
	ArchiveHistoryModeSummary ArchiveHistoryMode = "summary"
	ArchiveHistoryModeRaw     ArchiveHistoryMode = "raw"
	ArchiveHistoryModeBoth    ArchiveHistoryMode = "both"
)

type ImportOptions struct {
	SessionID          string
	Cwd                string
	HomeDir            string
	Agent              string
	CommandFingerprint string
}

func ExportSession(store *Store, id, outputPath string, opts ExportOptions) (err error) {
	if store == nil {
		return errors.New("acp client store is required")
	}
	if id == "" {
		return errors.New("acp client session id is required")
	}
	if outputPath == "" {
		return errors.New("archive output path is required")
	}
	lease, err := store.AcquireOwnerLease(OwnerLock{
		SessionID: id, PID: os.Getpid(), CommandFingerprint: "archive-export", StartedAt: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, ErrOwnerLeaseBusy) {
			return fmt.Errorf("%w: %s", ErrSessionActive, id)
		}
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	now := archiveNow(opts.Now)
	cwdRelative := cwdRelativeToHome(rec.Cwd, archiveHomeDir(opts.HomeDir))
	archive := struct {
		FormatVersion  int                   `json:"format_version"`
		ExportedAt     string                `json:"exported_at"`
		ExportedBy     string                `json:"exported_by"`
		Session        ArchiveSession        `json:"session"`
		History        any                   `json:"history"`
		SummaryHistory []ArchiveHistoryEvent `json:"summary_history,omitempty"`
	}{
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
	}
	summaryHistory := archiveHistory(rec)
	switch archiveHistoryMode(opts.HistoryMode) {
	case ArchiveHistoryModeSummary:
		archive.History = summaryHistory
	case ArchiveHistoryModeRaw:
		rawHistory, err := rawArchiveHistory(store, id)
		if err != nil {
			return err
		}
		archive.History = rawHistory
	case ArchiveHistoryModeBoth:
		rawHistory, err := rawArchiveHistory(store, id)
		if err != nil {
			return err
		}
		archive.History = rawHistory
		archive.SummaryHistory = summaryHistory
	default:
		return fmt.Errorf("unsupported archive history mode: %s", opts.HistoryMode)
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
	var events []EventLogLine
	if len(archive.RawHistory) > 0 {
		events = make([]EventLogLine, 0, len(archive.RawHistory))
		at := parseArchiveTime(archive.ExportedAt)
		if at.IsZero() {
			at = rec.UpdatedAt
		}
		for i, message := range archive.RawHistory {
			if err := ValidateJSONRPCMessage(message); err != nil {
				return SessionRecord{}, fmt.Errorf("%w: %v", ErrInvalidSessionArchive, err)
			}
			events = append(events, EventLogLine{
				Seq:       i + 1,
				At:        at,
				Direction: EventDirectionInbound,
				Message:   message,
			})
		}
	}
	if err := store.insertSessionWithEventLog(rec, events); err != nil {
		return SessionRecord{}, err
	}
	return store.Get(rec.ID)
}

func (s *Store) insertSessionWithEventLog(rec SessionRecord, events []EventLogLine) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	eventData, err := encodeEventLog(events, 1)
	if err != nil {
		return err
	}
	if s.beforeMutation != nil {
		s.beforeMutation()
	}
	return s.withFileLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if err := insertSessionRecord(&data, rec); err != nil {
			return err
		}
		return s.withEventLogLock(rec.ID, func(path string) error {
			if len(eventData) == 0 {
				if err := backgroundRemoveFile(path); err != nil {
					return err
				}
			} else if err := backgroundWriteFileAtomic(path, eventData); err != nil {
				return err
			}
			if err := s.saveUnlocked(data); err != nil {
				return errors.Join(err, backgroundRemoveFile(path))
			}
			return nil
		})
	})
}

func (a *Archive) UnmarshalJSON(b []byte) error {
	type archiveJSON struct {
		FormatVersion  int                   `json:"format_version"`
		ExportedAt     string                `json:"exported_at"`
		ExportedBy     string                `json:"exported_by"`
		Session        ArchiveSession        `json:"session"`
		History        []json.RawMessage     `json:"history"`
		SummaryHistory []ArchiveHistoryEvent `json:"summary_history"`
	}
	var src archiveJSON
	if err := json.Unmarshal(b, &src); err != nil {
		return err
	}
	acpxArchive := strings.EqualFold(strings.TrimSpace(src.ExportedBy), "acpx")
	*a = Archive{
		FormatVersion:  src.FormatVersion,
		ExportedAt:     src.ExportedAt,
		ExportedBy:     src.ExportedBy,
		Session:        src.Session,
		SummaryHistory: src.SummaryHistory,
	}
	for _, raw := range src.History {
		if err := ValidateJSONRPCMessage(raw); err == nil {
			a.RawHistory = append(a.RawHistory, raw)
			continue
		}
		if acpxArchive {
			return fmt.Errorf("%w: invalid acpx raw history event", ErrInvalidSessionArchive)
		}
		var event ArchiveHistoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return fmt.Errorf("%w: invalid history event: %v", ErrInvalidSessionArchive, err)
		}
		if strings.TrimSpace(event.Kind) == "" {
			return fmt.Errorf("%w: invalid history event", ErrInvalidSessionArchive)
		}
		a.History = append(a.History, event)
	}
	if len(a.History) == 0 && len(a.SummaryHistory) > 0 {
		a.History = append(a.History, a.SummaryHistory...)
	}
	return nil
}

func archiveHistoryMode(mode ArchiveHistoryMode) ArchiveHistoryMode {
	switch mode {
	case "", ArchiveHistoryModeSummary:
		return ArchiveHistoryModeSummary
	case ArchiveHistoryModeRaw:
		return ArchiveHistoryModeRaw
	case ArchiveHistoryModeBoth:
		return ArchiveHistoryModeBoth
	default:
		return mode
	}
}

func rawArchiveHistory(store *Store, id string) ([]json.RawMessage, error) {
	events, err := store.ReadEventLog(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrRawHistoryUnavailable, id)
		}
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrRawHistoryUnavailable, id)
	}
	history := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			return nil, err
		}
		history = append(history, event.Message)
	}
	return history, nil
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
