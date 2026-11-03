package gameservertest

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
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

// SlowTaskLogs returns the sim pool watchdog's "slow task" lines whose task
// ran for at least atLeast. The watchdog names every queue whose task ran
// past the 50 ms budget; a suite that slows its stores (WithSlowStores)
// passes the store delay, since a task that waited on one of those stores
// ran at least that long, while a shorter slow task is CPU contention on a
// loaded runner. A line whose elapsed time cannot be read is returned too.
func (s *Server) SlowTaskLogs(atLeast time.Duration) []string {
	var out []string
	for _, line := range strings.Split(s.LogText(), "\n") {
		if !strings.Contains(line, "sim: slow task") {
			continue
		}
		var entry struct {
			Elapsed *float64 `json:"elapsed"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Elapsed == nil ||
			time.Duration(*entry.Elapsed*float64(zerolog.DurationFieldUnit)) >= atLeast {
			out = append(out, line)
		}
	}
	return out
}
