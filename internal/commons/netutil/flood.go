package netutil

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// FloodGuardConfig carries the per-IP connection throttle settings read
// from the login server properties.
type FloodGuardConfig struct {
	Enabled              bool
	FastConnectionLimit  int
	NormalConnectionTime time.Duration
	FastConnectionTime   time.Duration
	MaxConnectionsPerIP  int
}

// foreignConnection tracks one remote IP's recent connection attempts.
type foreignConnection struct {
	attempts       int
	lastConnection time.Time
	flooding       bool
}

// FloodGuard drops fast-repeat connections per remote IP: more connections
// than FastConnectionLimit within NormalConnectionTime, any connection
// within FastConnectionTime of the previous one, or more than
// MaxConnectionsPerIP live attempts are rejected until the pace slows.
// Each accepted connection must call Release when it finishes so its IP's
// attempt count decays back down.
type FloodGuard struct {
	cfg FloodGuardConfig
	log zerolog.Logger

	mu    sync.Mutex
	conns map[string]*foreignConnection
}

// NewFloodGuard returns a FloodGuard applying cfg.
func NewFloodGuard(cfg FloodGuardConfig, log zerolog.Logger) *FloodGuard {
	return &FloodGuard{
		cfg:   cfg,
		log:   log,
		conns: make(map[string]*foreignConnection),
	}
}

// CanConnect decides whether a connection from ip at now is admitted,
// recording the attempt when it is.
func (g *FloodGuard) CanConnect(ip string, now time.Time) bool {
	if !g.cfg.Enabled {
		return true
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	fc, ok := g.conns[ip]
	if !ok {
		g.conns[ip] = &foreignConnection{attempts: 1, lastConnection: now}
		return true
	}

	fc.attempts++
	sinceLast := now.Sub(fc.lastConnection)
	if (fc.attempts > g.cfg.FastConnectionLimit && sinceLast < g.cfg.NormalConnectionTime) ||
		sinceLast < g.cfg.FastConnectionTime ||
		fc.attempts > g.cfg.MaxConnectionsPerIP {
		fc.lastConnection = now
		fc.attempts--
		if !fc.flooding {
			g.log.Info().Str("ip", ip).Msg("flood detected")
		}
		fc.flooding = true
		return false
	}

	if fc.flooding {
		fc.flooding = false
		g.log.Info().Str("ip", ip).Msg("no longer considered as flooding")
	}
	fc.lastConnection = now
	return true
}

// Release records that one accepted connection from ip finished, decaying
// its attempt count; the entry disappears once nothing is held anymore.
func (g *FloodGuard) Release(ip string) {
	if !g.cfg.Enabled {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if fc := g.conns[ip]; fc != nil {
		fc.attempts--
		if fc.attempts == 0 {
			delete(g.conns, ip)
		}
	}
}
