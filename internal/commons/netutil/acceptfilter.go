package netutil

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"
)

// The accept filter's limits are fixed; no configuration key changes them,
// and flood protection being disabled does not turn the filter off.
const (
	// acceptBurstWindow is how close to the previous connection a new one
	// must come to count toward the burst.
	acceptBurstWindow = time.Second
	// acceptBurstLimit is how many burst connections an address may open
	// after its first one; the next burst connection bans it.
	acceptBurstLimit = 3
	// acceptForgetAfter is how long an address must stay silent before the
	// sweep forgets it, lifting its ban.
	acceptForgetAfter = 300 * time.Second
	// acceptSweepInterval is how often a filtered listener sweeps.
	acceptSweepInterval = 5 * time.Second
)

// acceptHolder is one remote address's connection record.
type acceptHolder struct {
	lastAccess time.Time
	tries      int
	banned     bool
}

// AcceptFilter refuses remote addresses that open connections in fast
// bursts. Every connection from an address within acceptBurstWindow of its
// previous one counts as a try; the try after acceptBurstLimit bans the
// address. A slower connection is admitted but does not clear the tries.
// Only Sweep forgets an address, once it has been silent for
// acceptForgetAfter, so a ban lasts as long as the attempts keep coming.
//
// mu guards holders.
type AcceptFilter struct {
	mu      sync.Mutex
	holders map[netip.Addr]*acceptHolder
}

// NewAcceptFilter returns an AcceptFilter that knows no address.
func NewAcceptFilter() *AcceptFilter {
	return &AcceptFilter{holders: make(map[netip.Addr]*acceptHolder)}
}

// Allow records a connection from addr at now and reports whether it is
// admitted.
func (f *AcceptFilter) Allow(addr netip.Addr, now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	h, ok := f.holders[addr]
	if !ok {
		f.holders[addr] = &acceptHolder{lastAccess: now}
		return true
	}
	burst := h.lastAccess.Add(acceptBurstWindow).After(now)
	h.lastAccess = now
	switch {
	case h.banned:
		return false
	case !burst:
		return true
	case h.tries >= acceptBurstLimit:
		h.banned = true
		return false
	default:
		h.tries++
		return true
	}
}

// Sweep forgets every address whose last connection is more than
// acceptForgetAfter before now.
func (f *AcceptFilter) Sweep(now time.Time) {
	cutoff := now.Add(-acceptForgetAfter)
	f.mu.Lock()
	defer f.mu.Unlock()
	for addr, h := range f.holders {
		if h.lastAccess.Before(cutoff) {
			delete(f.holders, addr)
		}
	}
}

// FilterAccepts returns ln with f applied at accept: a connection f refuses,
// timed by now, is closed before a byte is read or written, and Accept
// waits for the next one. The returned listener sweeps f every
// acceptSweepInterval on a goroutine it owns, which stops once the listener
// is closed or ln reports itself closed. A connection whose remote address
// is not an IP address is admitted.
func FilterAccepts(ln net.Listener, f *AcceptFilter, now func() time.Time) net.Listener {
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
