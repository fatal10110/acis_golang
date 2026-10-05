package network

import (
	"sync"
	"sync/atomic"
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
	log atomic.Pointer[zerolog.Logger]
	now func() time.Time

	mu         sync.Mutex
	next       time.Time // earliest time the next diagnostic may be logged
	suppressed int       // rejections since the last diagnostic, not yet logged
}

func newFrameRejectReporter(now func() time.Time) *frameRejectReporter {
	return &frameRejectReporter{now: now}
}

// outboundRejects is the process-wide reporter every outbound send path
// reports to. Its log is unset, so diagnostics are dropped, until
// SetOutboundRejectLog installs one.
var outboundRejects = newFrameRejectReporter(time.Now)

// SetOutboundRejectLog directs the diagnostics for outbound frames refused
// before reaching the wire — invalid broadcast sources, too-short frames, and
// frames carrying an error — to log. The composition root calls it once at
// boot; until then those diagnostics are dropped.
func SetOutboundRejectLog(log zerolog.Logger) {
	outboundRejects.setLog(log)
}

func (r *frameRejectReporter) setLog(log zerolog.Logger) {
	r.log.Store(&log)
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

	if log := r.log.Load(); log != nil {
		log.Error().Err(cause).Str("boundary", boundary).Int("suppressed", suppressed).
			Msg("outbound frame rejected")
	}
}
