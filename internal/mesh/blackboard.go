package mesh

import (
	"sync"
	"time"
)

// Entry is a single value stored in a Blackboard section.
type Entry struct {
	Key       string
	Value     any
	Author    string // node ID that wrote it
	Revision  int64  // monotonic, for conflict detection
	Timestamp time.Time
}

// Section groups related entries under a named namespace.
type Section struct {
	Entries map[string]Entry
}

// Blackboard is a shared, thread-safe key-value store used by mesh nodes
// to exchange structured state. Sections partition the keyspace, and a
// global monotonic revision counter enables conflict detection.
type Blackboard struct {
	mu       sync.RWMutex
	sections map[string]*Section
	watchers []func(key string, val Entry)
	revision int64
}

// NewBlackboard returns an empty Blackboard ready for use.
func NewBlackboard() *Blackboard {
	return &Blackboard{
		sections: make(map[string]*Section),
	}
}

// Read returns the entry for section/key. The second return value is false
// if the section or key does not exist.
func (b *Blackboard) Read(section, key string) (Entry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sec, ok := b.sections[section]
	if !ok {
		return Entry{}, false
	}
	e, ok := sec.Entries[key]
	return e, ok
}

// Write stores a value under section/key, stamping it with author and an
// incremented global revision. Watchers are notified after the lock is released.
func (b *Blackboard) Write(section, key string, value any, author string) Entry {
	b.mu.Lock()
	sec, ok := b.sections[section]
	if !ok {
		sec = &Section{Entries: make(map[string]Entry)}
		b.sections[section] = sec
	}
	b.revision++
	e := Entry{
		Key:       key,
		Value:     value,
		Author:    author,
		Revision:  b.revision,
		Timestamp: time.Now(),
	}
	sec.Entries[key] = e

	// snapshot watchers under the lock so the slice is safe to iterate
	watchers := make([]func(string, Entry), len(b.watchers))
	copy(watchers, b.watchers)
	b.mu.Unlock()

	for _, fn := range watchers {
		if fn != nil {
			fn(section+"/"+key, e)
		}
	}
	return e
}

// List returns all entries in a section. Returns nil if the section does not exist.
func (b *Blackboard) List(section string) map[string]Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sec, ok := b.sections[section]
	if !ok {
		return nil
	}
	out := make(map[string]Entry, len(sec.Entries))
	for k, v := range sec.Entries {
		out[k] = v
	}
	return out
}

// ListSections returns the names of all sections that currently have entries.
func (b *Blackboard) ListSections() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.sections))
	for k := range b.sections {
		names = append(names, k)
	}
	return names
}

// WatcherID is a handle for removing a watcher.
type WatcherID int

// Watch registers a callback that is invoked after every Write.
// The callback receives the composite key ("section/key") and the entry.
// Returns a WatcherID that can be passed to Unwatch to remove the callback.
func (b *Blackboard) Watch(fn func(key string, val Entry)) WatcherID {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := WatcherID(len(b.watchers))
	b.watchers = append(b.watchers, fn)
	return id
}

// Unwatch removes a previously registered watcher by setting it to nil.
// Safe to call multiple times with the same ID.
func (b *Blackboard) Unwatch(id WatcherID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if int(id) < len(b.watchers) {
		b.watchers[id] = nil
	}
}

// WriteFromRemote stores a value without triggering watchers. Used to apply
// remote blackboard syncs without creating echo loops.
func (b *Blackboard) WriteFromRemote(section, key string, value any, author string, revision int64) Entry {
	b.mu.Lock()
	sec, ok := b.sections[section]
	if !ok {
		sec = &Section{Entries: make(map[string]Entry)}
		b.sections[section] = sec
	}
	if revision > b.revision {
		b.revision = revision
	} else {
		b.revision++
		revision = b.revision
	}
	e := Entry{
		Key:       key,
		Value:     value,
		Author:    author,
		Revision:  revision,
		Timestamp: time.Now(),
	}
	sec.Entries[key] = e
	b.mu.Unlock()
	// No watcher notification — prevents echo loops.
	return e
}
