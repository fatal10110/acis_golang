package sim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// blocker's methods each run until the blocker is closed, so the watchdog
// sees them still running and then slow.
type blocker chan struct{}

func (b blocker) posted()     { <-b }
func (b blocker) attributed() { <-b }
func (b blocker) timed()      { <-b }

func TestPoolWatchdogNamesTheWorkATaskRuns(t *testing.T) {
	var log lockedBuffer
	p := NewPool(1, zerolog.New(&log))
	p.Start(context.Background())
	q := p.NewQueue("player-7")

	cases := []struct {
		task  string
		start func(blocker)
	}{
		{"sim.blocker.posted-fm", func(b blocker) { q.Post(b.posted) }},
		// A wrapper task names the work it wraps, not itself.
		{"sim.blocker.attributed-fm", func(b blocker) {
			q.Post(func() {
				Attribute(q, b.attributed)
				b.attributed()
			})
		}},
		// A timer expiry names the timer's callback.
		{"sim.blocker.timed-fm", func(b blocker) { q.After(time.Millisecond, b.timed) }},
	}
	for _, c := range cases {
		b := make(blocker)
		c.start(b)
		waitForLogLine(t, &log, "sim: task still running", `"task":"`+c.task+`"`)
		close(b)
	}
	stopPool(t, p)
	for _, c := range cases {
		waitForLogLine(t, &log, "sim: slow task", `"task":"`+c.task+`"`)
	}
}

// waitForLogLine waits until one line of log holds every one of parts.
func waitForLogLine(t *testing.T, log *lockedBuffer, parts ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for line := range strings.SplitSeq(log.String(), "\n") {
			if containsAll(line, parts) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no log line holding %q:\n%s", parts, log.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
