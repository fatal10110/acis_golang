package gameservertest

import (
	"bytes"
	"strings"
	"sync"
)

// lockedBuffer collects log lines from every goroutine the server runs.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// LogText returns everything the server has logged so far, empty unless the
// suite booted with WithCapturedLog.
func (s *Server) LogText() string {
	if s.logs == nil {
		return ""
	}
	return s.logs.String()
}

// SlowTaskLogs returns the sim pool watchdog's "slow task" lines, which name
// a queue whose task ran past the 50 ms budget — a queued task that blocked,
// most likely on the database.
func (s *Server) SlowTaskLogs() []string {
	var out []string
	for _, line := range strings.Split(s.LogText(), "\n") {
		if strings.Contains(line, "sim: slow task") {
			out = append(out, line)
		}
	}
	return out
}
