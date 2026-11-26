package network

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// frameRejectInterval is the shortest gap between two rejected-frame
// diagnostics. Rejections inside it are counted, and the next diagnostic
// carries that count, so a broken frame sent in a hot loop costs one log line
// a minute rather than one per send or per recipient.
const frameRejectInterval = time.Minute

// frameRejectReporter is the one boundary that reports outbound frames
// refused before reaching the wire: a broadcast source no recipient copy can
// be made of, and a frame a session or connection will not queue. A refused
// frame is a server bug — some client misses a packet — so it must be
// visible, but bounded.
type frameRejectReporter struct {
	log zerolog.Logger
	now func() time.Time

	mu         sync.Mutex
	next       time.Time // earliest time the next diagnostic may be logged
	suppressed int       // rejections since the last diagnostic, not yet logged
}

// newFrameRejectReporter returns a reporter logging to log, with its own
// rate-limit window. A game-client link owns one for its broadcasts and a
// listener one for its connections' sends.
func newFrameRejectReporter(log zerolog.Logger, now func() time.Time) *frameRejectReporter {
	return &frameRejectReporter{log: log, now: now}
}

// report records one refused frame and why. boundary names the send path
// that refused it. It logs at most once per frameRejectInterval.
func (r *frameRejectReporter) report(boundary string, cause error) {
	r.mu.Lock()
	now := r.now()
	if now.Before(r.next) {
		r.suppressed++
		r.mu.Unlock()
		return
	}
	suppressed := r.suppressed
	r.suppressed = 0
	r.next = now.Add(frameRejectInterval)
	r.mu.Unlock()

	r.log.Error().Err(cause).Str("boundary", boundary).Int("suppressed", suppressed).
		Msg("outbound frame rejected")
}
