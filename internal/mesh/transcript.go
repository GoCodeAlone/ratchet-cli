package mesh

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// TranscriptLogger records BB writes and mesh messages to a writer.
type TranscriptLogger struct {
	w         io.Writer
	mu        sync.Mutex
	bb        *Blackboard
	watcherID WatcherID
	teamID    string
	start     time.Time
	writes    int
}

// NewTranscriptLogger creates a logger that watches bb writes and writes
// formatted transcript lines to w.
func NewTranscriptLogger(w io.Writer, bb *Blackboard, _ *Router, teamID string) *TranscriptLogger {
	tl := &TranscriptLogger{
		w:      w,
		bb:     bb,
		teamID: teamID,
		start:  time.Now(),
	}
	tl.watcherID = bb.Watch(func(key string, val Entry) {
		tl.mu.Lock()
		defer tl.mu.Unlock()
		tl.writes++
		elapsed := time.Since(tl.start)
		// Truncate value for log line.
		v := fmt.Sprintf("%v", val.Value)
		if len(v) > 200 {
			v = v[:200] + "..."
		}
		fmt.Fprintf(tl.w, "[%s] BB WRITE %s by %s rev=%d\n          | %s\n",
			formatElapsed(elapsed), key, val.Author, val.Revision, v)
	})
	return tl
}

// LogMessage records a mesh message to the transcript.
func (tl *TranscriptLogger) LogMessage(msg Message) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	elapsed := time.Since(tl.start)
	content := msg.Content
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	fmt.Fprintf(tl.w, "[%s] MSG %s → %s (%s)\n          | %s\n",
		formatElapsed(elapsed), msg.From, msg.To, msg.Type, content)
}

// LogStart records the team start event.
func (tl *TranscriptLogger) LogStart(task string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.start = time.Now()
	fmt.Fprintf(tl.w, "[00:00.0] TEAM %s STARTED — task: %q\n", tl.teamID, task)
}

// LogComplete records the team completion event.
func (tl *TranscriptLogger) LogComplete(agentCount, _ int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	elapsed := time.Since(tl.start)
	fmt.Fprintf(tl.w, "[%s] TEAM %s COMPLETED — %s, %d agents, %d BB writes\n",
		formatElapsed(elapsed), tl.teamID, elapsed.Round(100*time.Millisecond), agentCount, tl.writes)
}

// Stop removes the BB watcher.
func (tl *TranscriptLogger) Stop() {
	tl.bb.Unwatch(tl.watcherID)
}

// Writes returns the total number of BB writes observed.
func (tl *TranscriptLogger) Writes() int {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.writes
}

func formatElapsed(d time.Duration) string {
	totalSec := d.Seconds()
	min := int(totalSec) / 60
	sec := totalSec - float64(min*60)
	return fmt.Sprintf("%02d:%04.1f", min, sec)
}
