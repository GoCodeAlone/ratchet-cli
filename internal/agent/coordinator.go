package agent

import (
	"path/filepath"
	"sync"
)

// SessionRegistry is the interface for looking up workspace sessions.
type SessionRegistry interface {
	SessionsInDir(dir string) []string
}

// Coordinator manages workspace awareness across sessions.
type Coordinator struct {
	sessions SessionRegistry
	mu       sync.RWMutex
	// fileLocks maps file paths to session IDs currently writing
	fileLocks map[string]string
}

func NewCoordinator(sessions SessionRegistry) *Coordinator {
	return &Coordinator{
		sessions:  sessions,
		fileLocks: make(map[string]string),
	}
}

// CheckFileConflict returns session IDs of other sessions in the same workspace directory.
func (c *Coordinator) CheckFileConflict(sessionID, filePath string) []string {
	dir := filepath.Dir(filePath)
	all := c.sessions.SessionsInDir(dir)
	var conflicts []string
	for _, id := range all {
		if id != sessionID {
			conflicts = append(conflicts, id)
		}
	}
	return conflicts
}

// LockFile marks a file as being written by a session.
func (c *Coordinator) LockFile(sessionID, filePath string) {
	c.mu.Lock()
	c.fileLocks[filePath] = sessionID
	c.mu.Unlock()
}

// UnlockFile releases a file lock.
func (c *Coordinator) UnlockFile(filePath string) {
	c.mu.Lock()
	delete(c.fileLocks, filePath)
	c.mu.Unlock()
}

// FileLockHolder returns the session ID that holds the lock, or empty string.
func (c *Coordinator) FileLockHolder(filePath string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fileLocks[filePath]
}

// InjectCoordinationContext builds awareness messages for agent prompts.
func (c *Coordinator) InjectCoordinationContext(sessionID, workingDir string) string {
	peers := c.sessions.SessionsInDir(workingDir)
	if len(peers) <= 1 {
		return ""
	}
	return "Note: Other ratchet sessions are active in this workspace. Coordinate changes carefully."
}
