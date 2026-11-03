// Package shutdown runs the game master's timed server shutdown and
// restart: a per-second countdown that warns every player online, closes
// the login server's list to new logins in its last minute, and then stops
// the server.
package shutdown

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
)

// closeLoginsAt is the seconds left from which the login server lists the
// server as down.
const closeLoginsAt = 60

// Notifier tells every player online about the countdown.
type Notifier interface {
	// ServerDownIn warns that the server stops in seconds.
	ServerDownIn(seconds int32)
	// Announce says text on the announcement channel.
	Announce(text string, critical bool)
}

// LoginStatus is the status the login server lists the server under.
type LoginStatus interface {
	Type() link.ServerType
	SetType(link.ServerType)
	// GMOnly reports whether the server is reserved for game masters, the
	// status an aborted countdown restores instead of the open one.
	GMOnly() bool
}

// Countdown is the server's one shutdown countdown. Its state is guarded
// by mu; its ticks run on q.
type Countdown struct {
	q      *sim.Queue
	notify Notifier
	status LoginStatus
	stop   func(restart bool)
	log    zerolog.Logger

	mu      sync.Mutex
	started bool     // a countdown was ever started
	restart bool     // the mode the last countdown was started in
	current *counter // the last countdown started, nil before the first
}

// counter is one countdown. Every field is guarded by Countdown.mu.
type counter struct {
	seconds int32 // left before the stop
	restart bool
	aborted bool
	done    bool
	ticker  *sim.Ticker
}

// New returns the countdown, ticking on q. stop ends the process, asking
// for a restart when restart is set.
func New(q *sim.Queue, notify Notifier, status LoginStatus, stop func(restart bool), log zerolog.Logger) *Countdown {
	return &Countdown{q: q, notify: notify, status: status, stop: stop, log: log}
}

// warned reports whether the countdown warns players with seconds left.
func warned(seconds int32) bool {
	switch seconds {
	case 540, 480, 420, 360, 300, 240, 180, 120, 60, 30, 10, 5, 4, 3, 2, 1:
		return true
	}
	return false
}

// modeText names a countdown's mode in the log and the abort notice.
func modeText(restart bool) string {
	if restart {
		return "restarting"
	}
	return "shutting down"
}

// Start begins a countdown of seconds, by issuer, that restarts the server
// when restart is set and shuts it down otherwise. A countdown already
// running is aborted. Players are warned at once when seconds is not one
// of the countdown's own warnings; a count of zero or less stops at
// the first tick.
func (c *Countdown) Start(issuer string, seconds int32, restart bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restart = restart
	c.log.Info().Str("gm", issuer).Int32("seconds", seconds).Msgf("GM: %s issued %s process in %d seconds.", issuer, modeText(restart), seconds)
	if !warned(seconds) {
		c.notify.ServerDownIn(seconds)
	}
	if c.current != nil {
		c.current.aborted = true
	}
	n := &counter{seconds: max(0, seconds), restart: restart}
	c.started, c.current = true, n
	// The first tick runs right away, then one each second.
	c.q.Post(func() { c.tick(n) })
	n.ticker = c.q.Every(time.Second, func() { c.tick(n) })
}

// StartUnabortable begins a countdown of seconds that restarts the server
// when restart is set and shuts it down otherwise, beside the one Start
// runs: it aborts no countdown, Abort does not stop it, and a later Start
// leaves it running. Players are first warned at its first warning time,
// never at once; a count of zero or less stops at the first tick.
func (c *Countdown) StartUnabortable(seconds int32, restart bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := &counter{seconds: max(0, seconds), restart: restart}
	c.q.Post(func() { c.tick(n) })
	n.ticker = c.q.Every(time.Second, func() { c.tick(n) })
}

// Abort stops the running countdown, by issuer, and tells every player the
// server carries on. Once any countdown was started the notice is said
// even when none is running any more; before that Abort does nothing.
func (c *Countdown) Abort(issuer string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return
	}
	c.log.Info().Str("gm", issuer).Msgf("GM: %s aborted %s process.", issuer, modeText(c.restart))
	c.current.aborted = true
	c.notify.Announce("Server aborted "+modeText(c.restart)+" process and continues normal operation.", false)
}

// tick runs one second of n. An aborted countdown notices it on its next
// tick: it gives the login server its listing back if it had closed it, and
// ends. A countdown aborted during its last second neither restores the
// listing nor stops the server.
func (c *Countdown) tick(n *counter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n.done {
		return
	}
	if n.seconds <= 0 {
		n.done = true
		n.ticker.Stop()
		if !n.aborted {
			c.stop(n.restart)
		}
		return
	}
	if n.aborted {
		n.done = true
		n.ticker.Stop()
		if c.status.Type() == link.ServerTypeDown {
			if c.status.GMOnly() {
				c.status.SetType(link.ServerTypeGMOnly)
			} else {
				c.status.SetType(link.ServerTypeAuto)
			}
		}
		return
	}
	if warned(n.seconds) {
		c.notify.ServerDownIn(n.seconds)
	}
	if n.seconds <= closeLoginsAt && c.status.Type() != link.ServerTypeDown {
		c.status.SetType(link.ServerTypeDown)
	}
	n.seconds--
}
