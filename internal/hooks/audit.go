package hooks

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	maxHookAuditBytes      = 4 << 20
	defaultHookAuditLimit  = 100
	maxHookAuditReadLimit  = 1000
	hookAuditFileName      = "managed-hooks.jsonl"
	hookAuditDirectoryName = "audit"
	hookAuditArchiveSuffix = ".1"
)

var (
	// ErrManagedHookCommandFailed classifies managed execution failures without
	// exposing process errors or output.
	ErrManagedHookCommandFailed = errors.New("managed hook command failed")
	// ErrHookAuditDegraded classifies a required managed-hook audit failure.
	ErrHookAuditDegraded = errors.New("managed hook audit degraded")
	hookAuditUserHomeDir = os.UserHomeDir
)

// HookAuditResult is a metadata-only managed hook execution classification.
type HookAuditResult string

const (
	HookAuditStarted       HookAuditResult = "started"
	HookAuditSuccess       HookAuditResult = "success"
	HookAuditCommandFailed HookAuditResult = "command_failed"
	HookAuditDegraded      HookAuditResult = "audit_degraded"
)

type hookAuditWindowsAccessEntry struct {
	allowed     bool
	owner       bool
	fullControl bool
	inheritOnly bool
}

type hookAuditWindowsAnchorAccessEntry struct {
	allowed  bool
	trusted  bool
	mutating bool
}

func validateHookAuditWindowsAccess(ownerMatches, protected bool, entries []hookAuditWindowsAccessEntry) error {
	if !ownerMatches {
		return errors.New("managed hook audit owner is not the current user")
	}
	if !protected {
		return errors.New("managed hook audit DACL is not protected")
	}
	if len(entries) == 0 {
		return errors.New("managed hook audit DACL is empty")
	}
	for _, entry := range entries {
		if !entry.allowed || !entry.owner || !entry.fullControl || entry.inheritOnly {
			return errors.New("managed hook audit DACL is not owner-only full control")
		}
	}
	return nil
}

func validateHookAuditWindowsAnchorAccess(ownerTrusted, daclPresent bool, entries []hookAuditWindowsAnchorAccessEntry) error {
	if !ownerTrusted {
		return errors.New("managed hook audit trusted anchor owner is untrusted")
	}
	if !daclPresent {
		return errors.New("managed hook audit trusted anchor has a null DACL")
	}
	for _, entry := range entries {
		if entry.allowed && entry.mutating && !entry.trusted {
			return errors.New("managed hook audit trusted anchor grants mutation rights to an untrusted principal")
		}
	}
	return nil
}

// HookAuditRecord intentionally contains no executable, payload, output, or
// error text.
type HookAuditRecord struct {
	Timestamp  time.Time       `json:"timestamp"`
	Event      Event           `json:"event"`
	Hash       string          `json:"hash"`
	Source     SourceKind      `json:"source"`
	Result     HookAuditResult `json:"result"`
	DurationMS int64           `json:"duration_ms"`
}

// HookAuditWriter is the durable append boundary required before managed hook
// process launch.
type HookAuditWriter interface {
	Append(HookAuditRecord) error
}

// HookAudit stores managed hook metadata as owner-only JSONL.
type HookAudit struct {
	path                string
	syncFile            func(*os.File) error
	syncDir             func(string) error
	rotateFile          func(string, string) error
	beforeProcessLock   func() error
	afterProcessLock    func() error
	beforeProcessUnlock func() error
}

type hookAuditPathState struct {
	sync.Mutex
	degraded *HookAuditRecord
}

var hookAuditPathLocks sync.Map

// NewHookAudit creates a managed hook audit at path.
func NewHookAudit(path string) *HookAudit {
	return &HookAudit{path: filepath.Clean(path)}
}

// DefaultHookAuditPath returns the user-scoped managed hook audit path.
func DefaultHookAuditPath() (string, error) {
	home, err := hookAuditUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve managed hook audit home: %w", err)
	}
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", errors.New("resolve managed hook audit home: absolute path is required")
	}
	return filepath.Join(home, ".ratchet", hookAuditDirectoryName, hookAuditFileName), nil
}

// DefaultHookAuditReadLimit returns the operator CLI's default record limit.
func DefaultHookAuditReadLimit() int { return defaultHookAuditLimit }

// MaxHookAuditReadLimit returns the largest accepted operator read limit.
func MaxHookAuditReadLimit() int { return maxHookAuditReadLimit }

// Path returns the configured JSONL path.
func (a *HookAudit) Path() string {
	if a == nil {
		return ""
	}
	return a.path
}

// Append validates, appends, and syncs the requested record plus any pending
// degradation marker.
func (a *HookAudit) Append(record HookAuditRecord) (err error) {
	if err := validateHookAuditPath(a); err != nil {
		return err
	}
	if err := validateHookAuditRecord(record); err != nil {
		return err
	}
	lock := hookAuditPathLock(a.path)
	lock.Lock()
	defer lock.Unlock()
	defer func() {
		if err != nil && lock.degraded == nil {
			degraded := record
			degraded.Timestamp = time.Now().UTC()
			degraded.Result = HookAuditDegraded
			lock.degraded = &degraded
		}
	}()
	releaseAnchor, err := acquireHookAuditTrustedAnchor(a.path)
	if err != nil {
		return fmt.Errorf("validate managed hook audit trusted anchor: %w", err)
	}
	defer func() {
		err = errors.Join(err, releaseAnchor())
	}()
	releaseProcessLock, err := acquireHookAuditProcessLock(a.path, a.beforeProcessLock, a.afterProcessLock, a.beforeProcessUnlock)
	if err != nil {
		return fmt.Errorf("lock managed hook audit: %w", err)
	}
	defer func() {
		err = errors.Join(err, releaseProcessLock())
	}()
	record.Timestamp = record.Timestamp.UTC()
	records := []HookAuditRecord{record}
	if lock.degraded != nil {
		records = append([]HookAuditRecord{*lock.degraded}, records...)
	}
	line := make([]byte, 0, len(records)*192)
	for _, pending := range records {
		encoded, encodeErr := json.Marshal(pending)
		if encodeErr != nil {
			return fmt.Errorf("encode managed hook audit record: %w", encodeErr)
		}
		line = append(line, encoded...)
		line = append(line, '\n')
	}

	f, _, err := openHookAuditFile(a.path, true)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			err = errors.Join(err, f.Close())
		}
	}()
	if err := repairHookAuditTail(f, a.path, a.sync); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat managed hook audit: %w", err)
	}
	if len(line) > maxHookAuditBytes {
		return errors.New("managed hook audit record exceeds maximum size")
	}
	if info.Size() > maxHookAuditBytes-int64(len(line)) {
		current := f
		f = nil
		f, err = a.rotate(current)
		if err != nil {
			return err
		}
	}
	if err := validateHookAuditIdentity(a.path, f); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek managed hook audit: %w", err)
	}
	written, err := f.Write(line)
	if err == nil && written != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("append managed hook audit: %w", err)
	}
	if err := a.sync(f); err != nil {
		return fmt.Errorf("sync managed hook audit: %w", err)
	}
	for _, directory := range hookAuditNamespaceSyncDirectories(a.path) {
		if err := a.syncDirectory(directory); err != nil {
			return fmt.Errorf("sync managed hook audit namespace: %w", err)
		}
	}
	if err := validateHookAuditIdentity(a.path, f); err != nil {
		return err
	}
	lock.degraded = nil
	return nil
}

// Read returns at most limit committed records, newest first. A final record
// without a newline is treated as torn and ignored; malformed committed lines
// fail the read.
func (a *HookAudit) Read(limit int) (records []HookAuditRecord, err error) {
	if err := validateHookAuditPath(a); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxHookAuditReadLimit {
		return nil, fmt.Errorf("managed hook audit limit must be between 1 and %d", maxHookAuditReadLimit)
	}

	lock := hookAuditPathLock(a.path)
	lock.Lock()
	defer lock.Unlock()
	releaseAnchor, err := acquireHookAuditTrustedAnchor(a.path)
	if err != nil {
		return nil, fmt.Errorf("validate managed hook audit trusted anchor: %w", err)
	}
	defer func() {
		err = errors.Join(err, releaseAnchor())
	}()
	ready, err := prepareHookAuditPrivateNamespace(a.path, false)
	if err != nil {
		return nil, err
	}
	if !ready {
		return []HookAuditRecord{}, nil
	}
	hasGeneration, err := hookAuditHasGeneration(a.path)
	if err != nil {
		return nil, err
	}
	if !hasGeneration {
		return []HookAuditRecord{}, nil
	}
	releaseProcessLock, err := acquireHookAuditProcessLock(a.path, a.beforeProcessLock, a.afterProcessLock, a.beforeProcessUnlock)
	if err != nil {
		return nil, fmt.Errorf("lock managed hook audit: %w", err)
	}
	defer func() {
		err = errors.Join(err, releaseProcessLock())
	}()
	records, activeInfo, err := readHookAuditGeneration(a.path, limit)
	if err != nil || len(records) == limit {
		return records, err
	}
	archive, archiveInfo, err := readHookAuditGeneration(a.path+hookAuditArchiveSuffix, limit-len(records))
	if err != nil {
		return nil, err
	}
	if activeInfo != nil && archiveInfo != nil && os.SameFile(activeInfo, archiveInfo) {
		return records, nil
	}
	return append(records, archive...), nil
}

func validateHookAuditPath(a *HookAudit) error {
	if a == nil || strings.TrimSpace(a.path) == "" || a.path == "." {
		return errors.New("managed hook audit path is required")
	}
	if !filepath.IsAbs(a.path) {
		return errors.New("managed hook audit requires an absolute path")
	}
	_, _, err := hookAuditNamespace(a.path)
	return err
}

func hookAuditNamespace(path string) (string, [2]string, error) {
	inner := filepath.Dir(path)
	outer := filepath.Dir(inner)
	anchor := filepath.Dir(outer)
	if inner == path || outer == inner || anchor == outer {
		return "", [2]string{}, errors.New("managed hook audit path requires two namespace levels beneath a trusted anchor")
	}
	return anchor, [2]string{outer, inner}, nil
}

func hookAuditHasGeneration(path string) (bool, error) {
	for _, candidate := range []string{path, path + hookAuditArchiveSuffix} {
		if _, err := os.Lstat(candidate); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect managed hook audit generation: %w", err)
		}
	}
	return false, nil
}

func readHookAuditGeneration(path string, limit int) (records []HookAuditRecord, info os.FileInfo, err error) {
	f, _, err := openHookAuditFile(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return []HookAuditRecord{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err = f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat managed hook audit: %w", err)
	}
	if info.Size() > maxHookAuditBytes {
		return nil, nil, errors.New("managed hook audit exceeds maximum size")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxHookAuditBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read managed hook audit: %w", err)
	}
	if len(data) > maxHookAuditBytes {
		return nil, nil, errors.New("managed hook audit exceeds maximum size")
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		committed := bytes.LastIndexByte(data, '\n') + 1
		data = data[:committed]
	}
	if len(data) == 0 {
		return []HookAuditRecord{}, info, nil
	}
	records, err = decodeHookAuditRecords(data, limit)
	return records, info, err
}

func decodeHookAuditRecords(data []byte, limit int) ([]HookAuditRecord, error) {
	ring := make([]HookAuditRecord, limit)
	total := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4<<10), maxHookAuditBytes+1)
	for scanner.Scan() {
		line := scanner.Bytes()
		lineIndex := total + 1
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("read managed hook audit committed record %d: empty record", lineIndex)
		}
		record, err := decodeHookAuditRecord(line)
		if err != nil {
			return nil, fmt.Errorf("read managed hook audit committed record %d: %w", lineIndex, err)
		}
		ring[total%limit] = record
		total++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read managed hook audit: %w", err)
	}
	retained := min(total, limit)
	records := make([]HookAuditRecord, retained)
	for i := range retained {
		records[i] = ring[(total-1-i)%limit]
	}
	return records, nil
}

func decodeHookAuditRecord(line []byte) (HookAuditRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	token, err := decoder.Token()
	if err != nil {
		return HookAuditRecord{}, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return HookAuditRecord{}, errors.New("managed hook audit record must be an object")
	}
	var record HookAuditRecord
	seen := make(map[string]struct{}, 6)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return HookAuditRecord{}, err
		}
		key, ok := token.(string)
		if !ok {
			return HookAuditRecord{}, errors.New("managed hook audit field name must be a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return HookAuditRecord{}, fmt.Errorf("managed hook audit field %q is duplicated", key)
		}
		seen[key] = struct{}{}
		switch key {
		case "timestamp":
			err = decoder.Decode(&record.Timestamp)
		case "event":
			err = decoder.Decode(&record.Event)
		case "hash":
			err = decoder.Decode(&record.Hash)
		case "source":
			err = decoder.Decode(&record.Source)
		case "result":
			err = decoder.Decode(&record.Result)
		case "duration_ms":
			err = decoder.Decode(&record.DurationMS)
		default:
			return HookAuditRecord{}, fmt.Errorf("managed hook audit field %q is unknown", key)
		}
		if err != nil {
			return HookAuditRecord{}, fmt.Errorf("decode managed hook audit field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return HookAuditRecord{}, err
	}
	for _, required := range []string{"timestamp", "event", "hash", "source", "result", "duration_ms"} {
		if _, ok := seen[required]; !ok {
			return HookAuditRecord{}, fmt.Errorf("managed hook audit field %q is required", required)
		}
	}
	if err := requireJSONEOF(decoder); err != nil {
		return HookAuditRecord{}, err
	}
	if err := validateHookAuditRecord(record); err != nil {
		return HookAuditRecord{}, err
	}
	return record, nil
}

func (a *HookAudit) sync(f *os.File) error {
	if a.syncFile != nil {
		return a.syncFile(f)
	}
	return f.Sync()
}

func (a *HookAudit) syncDirectory(path string) error {
	if a.syncDir != nil {
		return a.syncDir(path)
	}
	return syncHookAuditDirectory(path)
}

func (a *HookAudit) rotatePath(source, destination string) error {
	if a.rotateFile != nil {
		return a.rotateFile(source, destination)
	}
	return rotateHookAuditPath(source, destination)
}

func hookAuditNamespaceSyncDirectories(path string) []string {
	anchor, namespace, err := hookAuditNamespace(path)
	if err != nil {
		return nil
	}
	return []string{namespace[1], namespace[0], anchor}
}

func (a *HookAudit) rotate(current *os.File) (_ *os.File, err error) {
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, current.Close())
		}
	}()
	if err := validateHookAuditIdentity(a.path, current); err != nil {
		return nil, err
	}
	if err := a.sync(current); err != nil {
		return nil, fmt.Errorf("sync managed hook audit before rotation: %w", err)
	}
	if err := current.Close(); err != nil {
		return nil, fmt.Errorf("close managed hook audit before rotation: %w", err)
	}
	closed = true

	archivePath := a.path + hookAuditArchiveSuffix
	archive, _, err := openHookAuditFile(archivePath, false)
	if err == nil {
		if err := archive.Close(); err != nil {
			return nil, fmt.Errorf("close managed hook audit archive: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect managed hook audit archive: %w", err)
	}
	if err := a.rotatePath(a.path, archivePath); err != nil {
		return nil, fmt.Errorf("rotate managed hook audit: %w", err)
	}
	if err := a.syncDirectory(filepath.Dir(a.path)); err != nil {
		return nil, fmt.Errorf("sync managed hook audit rotation: %w", err)
	}
	next, created, err := openHookAuditFile(a.path, true)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, errors.Join(errors.New("managed hook audit rotation did not create a new file"), next.Close())
	}
	return next, nil
}

func hookAuditPathLock(path string) *hookAuditPathState {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		abs = filepath.Clean(path)
	}
	lock, _ := hookAuditPathLocks.LoadOrStore(abs, new(hookAuditPathState))
	return lock.(*hookAuditPathState)
}

func repairHookAuditTail(f *os.File, path string, syncFile func(*os.File) error) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat managed hook audit: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	if info.Size() > maxHookAuditBytes {
		return errors.New("managed hook audit exceeds maximum size")
	}
	if _, err := f.Seek(-1, io.SeekEnd); err != nil {
		return fmt.Errorf("seek managed hook audit tail: %w", err)
	}
	var tail [1]byte
	if _, err := io.ReadFull(f, tail[:]); err != nil {
		return fmt.Errorf("read managed hook audit tail: %w", err)
	}
	if tail[0] == '\n' {
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek managed hook audit repair: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxHookAuditBytes+1))
	if err != nil {
		return fmt.Errorf("read managed hook audit repair: %w", err)
	}
	committed := bytes.LastIndexByte(data, '\n') + 1
	if err := validateHookAuditIdentity(path, f); err != nil {
		return err
	}
	if err := f.Truncate(int64(committed)); err != nil {
		return fmt.Errorf("repair managed hook audit tail: %w", err)
	}
	if err := syncFile(f); err != nil {
		return fmt.Errorf("sync managed hook audit repair: %w", err)
	}
	return nil
}

func validateHookAuditRecord(record HookAuditRecord) error {
	if record.Timestamp.IsZero() {
		return errors.New("managed hook audit timestamp is required")
	}
	if !slices.Contains(AllEvents, record.Event) {
		return errors.New("managed hook audit event is invalid")
	}
	decodedHash, err := hex.DecodeString(record.Hash)
	if err != nil || len(decodedHash) != 32 {
		return errors.New("managed hook audit hash is invalid")
	}
	if record.Source != SourceManaged {
		return errors.New("managed hook audit source must be managed")
	}
	switch record.Result {
	case HookAuditStarted, HookAuditSuccess, HookAuditCommandFailed, HookAuditDegraded:
	default:
		return errors.New("managed hook audit result is invalid")
	}
	if record.DurationMS < 0 {
		return errors.New("managed hook audit duration must not be negative")
	}
	if record.Result == HookAuditStarted && record.DurationMS != 0 {
		return errors.New("managed hook audit started duration must be zero")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}
