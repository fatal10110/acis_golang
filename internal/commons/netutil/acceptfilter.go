package netutil

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// acceptSweepInterval is how often a filtered listener sweeps.
const acceptSweepInterval = 5 * time.Second

// AcceptFilterConfig is how an AcceptFilter judges connection bursts.
type AcceptFilterConfig struct {
	// Enabled turns the filter on; FilterAccepts leaves the listener
	// unfiltered when it is off.
	Enabled bool
	// BurstWindow is how close to the previous connection a new one must
	// come to count toward the burst.
	BurstWindow time.Duration
	// BurstLimit is how many burst connections an address may open after
	// its first one; the next burst connection bans it.
	BurstLimit int
	// BanTime is how long an address must stay silent before the sweep
	// forgets it, lifting its ban.
	BanTime time.Duration
	// Allowlist holds the prefixes, IPv4 ones in IPv4 form, whose
	// addresses the filter always admits and never records.
	Allowlist []netip.Prefix
}

// DefaultAcceptFilterConfig is the filter both client ports run unless
// configured otherwise: on, a 1 s burst window, 3 burst connections
// allowed, forgotten after 300 s of silence, no allowlist.
func DefaultAcceptFilterConfig() AcceptFilterConfig {
	return AcceptFilterConfig{
		Enabled:     true,
		BurstWindow: time.Second,
		BurstLimit:  3,
		BanTime:     300 * time.Second,
	}
}

// acceptHolder is one remote address's connection record.
type acceptHolder struct {
	lastAccess time.Time
	tries      int
	banned     bool
}

// AcceptFilter refuses remote addresses that open connections in fast
// bursts. Every connection from an address within BurstWindow of its
// previous one counts as a try; the try after BurstLimit bans the address.
// A slower connection is admitted but does not clear the tries. Only Sweep
// forgets an address, once it has been silent for BanTime, so a ban lasts
// as long as the attempts keep coming.
//
// cfg and log are fixed at construction; mu guards holders.
type AcceptFilter struct {
	cfg AcceptFilterConfig
	log zerolog.Logger

	mu      sync.Mutex
	holders map[netip.Addr]*acceptHolder
}

// NewAcceptFilter returns an AcceptFilter on cfg that knows no address and
// logs each ban to log at warn level.
func NewAcceptFilter(cfg AcceptFilterConfig, log zerolog.Logger) *AcceptFilter {
	return &AcceptFilter{cfg: cfg, log: log, holders: make(map[netip.Addr]*acceptHolder)}
}

// Allow records a connection from addr at now and reports whether it is
// admitted.
func (f *AcceptFilter) Allow(addr netip.Addr, now time.Time) bool {
	if f.allowlisted(addr) {
		return true
	}
	allowed, banned := f.record(addr, now)
	if banned {
		f.log.Warn().Str("ip", addr.String()).Dur("burst_window", f.cfg.BurstWindow).
			Int("burst_limit", f.cfg.BurstLimit).Dur("ban_time", f.cfg.BanTime).
			Msg("connection burst; address banned at accept")
	}
	return allowed
}

func (f *AcceptFilter) allowlisted(addr netip.Addr) bool {
	for _, p := range f.cfg.Allowlist {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// record is Allow for an address off the allowlist; banned reports that
// this connection banned addr.
func (f *AcceptFilter) record(addr netip.Addr, now time.Time) (allowed, banned bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	h, ok := f.holders[addr]
	if !ok {
		f.holders[addr] = &acceptHolder{lastAccess: now}
		return true, false
	}
	burst := h.lastAccess.Add(f.cfg.BurstWindow).After(now)
	h.lastAccess = now
	switch {
	case h.banned:
		return false, false
	case !burst:
		return true, false
	case h.tries >= f.cfg.BurstLimit:
		h.banned = true
		return false, true
	default:
		h.tries++
		return true, false
	}
}

// Sweep forgets every address whose last connection is more than BanTime
// before now.
func (f *AcceptFilter) Sweep(now time.Time) {
	cutoff := now.Add(-f.cfg.BanTime)
	f.mu.Lock()
	defer f.mu.Unlock()
	for addr, h := range f.holders {
		if h.lastAccess.Before(cutoff) {
			delete(f.holders, addr)
		}
	}
}

// List returns every address f bans. A ban's Expires is BanTime after the
// address's last attempt: the first sweep past it lifts the ban, and each
// attempt before then pushes it back.
func (f *AcceptFilter) List() []BanEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	var bans []BanEntry
	for addr, h := range f.holders {
		if h.banned {
			bans = append(bans, BanEntry{Addr: addr, Expires: h.lastAccess.Add(f.cfg.BanTime)})
		}
	}
	return bans
}

// Remove lifts f's ban on addr and reports whether there was one. The
// address's record goes with it, so its next connection starts a new count.
// An address f is only counting tries for is left as it is.
func (f *AcceptFilter) Remove(addr netip.Addr) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.holders[addr]
	if !ok || !h.banned {
		return false
	}
	delete(f.holders, addr)
	return true
}

// FilterAccepts returns ln with f applied at accept: a connection f refuses,
// timed by now, is closed before a byte is read or written, and Accept
// waits for the next one. The returned listener sweeps f every
// acceptSweepInterval on a goroutine it owns, which stops once the listener
// is closed or ln reports itself closed. A connection whose remote address
// is not an IP address is admitted. A filter whose config is not Enabled
// leaves ln as it is.
func FilterAccepts(ln net.Listener, f *AcceptFilter, now func() time.Time) net.Listener {
	if !f.cfg.Enabled {
		return ln
	}
	ticker := time.NewTicker(acceptSweepInterval)
	return newFilteredListener(ln, f, now, ticker.C, ticker.Stop)
}

// filteredListener is the listener FilterAccepts returns. Its sweep
// goroutine runs until stop is closed and closes swept when it returns.
type filteredListener struct {
	net.Listener
	filter *AcceptFilter
	now    func() time.Time

	stopOnce sync.Once
	stop     chan struct{}
	swept    chan struct{}
}

// newFilteredListener is FilterAccepts sweeping on ticks; stopTicks, if not
// nil, runs once the sweeping ends.
func newFilteredListener(ln net.Listener, f *AcceptFilter, now func() time.Time, ticks <-chan time.Time, stopTicks func()) *filteredListener {
	l := &filteredListener{
		Listener: ln,
		filter:   f,
		now:      now,
		stop:     make(chan struct{}),
		swept:    make(chan struct{}),
	}
	go func() {
		defer close(l.swept)
		if stopTicks != nil {
			defer stopTicks()
		}
		for {
			select {
			case <-l.stop:
				return
			case <-ticks:
				f.Sweep(now())
			}
		}
	}()
	return l
}

// Accept returns the next connection the filter admits.
func (l *filteredListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				l.stopSweeping()
			}
			return nil, err
		}
		addr, ok := remoteAddr(conn)
		if !ok || l.filter.Allow(addr, l.now()) {
			return conn, nil
		}
		conn.Close()
	}
}

// Close closes the listener and waits for the sweeping to stop.
func (l *filteredListener) Close() error {
	err := l.Listener.Close()
	l.stopSweeping()
	return err
}

func (l *filteredListener) stopSweeping() {
	l.stopOnce.Do(func() { close(l.stop) })
	<-l.swept
}

// remoteAddr is conn's remote IP address, an IPv4-mapped IPv6 address
// reduced to its IPv4 form, so one host has one key whichever way it
// reaches a dual-stack listener.
func remoteAddr(conn net.Conn) (netip.Addr, bool) {
	tcp, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(tcp.IP)
	return addr.Unmap(), ok
}
