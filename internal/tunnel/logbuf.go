package tunnel

import "sync"

// logBuffer is a fixed-capacity ring of the most recent log lines. It keeps
// the last cloudflared output around for the dashboard debug console even
// after the process exits or is stopped.
type logBuffer struct {
	mu    sync.Mutex
	lines []string
	cap   int
}

func newLogBuffer(capacity int) *logBuffer {
	return &logBuffer{cap: capacity}
}

func (b *logBuffer) add(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.cap {
		b.lines = b.lines[len(b.lines)-b.cap:]
	}
}

func (b *logBuffer) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

func (b *logBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = nil
}
