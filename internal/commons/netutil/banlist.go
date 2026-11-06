package netutil

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// BanEntry is one banned address. Expires is when the ban lifts; the zero
// time means it never does.
type BanEntry struct {
	Addr    netip.Addr
	Expires time.Time
}

// Permanent reports whether the ban never lifts.
func (e BanEntry) Permanent() bool { return e.Expires.IsZero() }

// BanList holds addresses banned until a set time or for good. A ban whose
// time has passed is dropped the next time it is checked or listed.
//
// now is fixed at construction; mu guards bans.
type BanList struct {
	now func() time.Time

	mu   sync.Mutex
	bans map[netip.Addr]time.Time // zero time: permanent
}

// NewBanList returns an empty BanList timed by now.
func NewBanList(now func() time.Time) *BanList {
	return &BanList{now: now, bans: make(map[netip.Addr]time.Time)}
}

// Add bans addr for d, or for good when d <= 0, and reports whether it did.
// An address already banned keeps its ban and expiry.
func (b *BanList) Add(addr netip.Addr, d time.Duration) bool {
	addr = addr.Unmap()
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if b.liveLocked(addr, now) {
		return false
	}
	var until time.Time
	if d > 0 {
		until = now.Add(d)
	}
	b.bans[addr] = until
	return true
}

// Remove lifts the ban on addr and reports whether there was one.
func (b *BanList) Remove(addr netip.Addr) bool {
	addr = addr.Unmap()
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.liveLocked(addr, b.now()) {
		return false
	}
	delete(b.bans, addr)
	return true
}

// IsBanned reports whether addr is banned now.
func (b *BanList) IsBanned(addr netip.Addr) bool {
	addr = addr.Unmap()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.liveLocked(addr, b.now())
}

// List returns every address banned now.
func (b *BanList) List() []BanEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	var bans []BanEntry
	for addr := range b.bans {
		if b.liveLocked(addr, now) {
			bans = append(bans, BanEntry{Addr: addr, Expires: b.bans[addr]})
		}
	}
	return bans
}

// liveLocked reports whether addr is banned at now, dropping a ban whose
// time has passed. b.mu must be held.
func (b *BanList) liveLocked(addr netip.Addr, now time.Time) bool {
	until, ok := b.bans[addr]
	if !ok {
		return false
	}
	if !until.IsZero() && now.After(until) {
		delete(b.bans, addr)
		return false
	}
	return true
}

// RefuseBanned returns ln closing, before a byte is read or written, every
// connection whose remote address bans holds; Accept waits for the next
// one. A connection whose remote address is not an IP address is admitted.
func RefuseBanned(ln net.Listener, bans *BanList) net.Listener {
	return &banListener{Listener: ln, bans: bans}
}

type banListener struct {
	net.Listener
	bans *BanList
}

// Accept returns the next connection from an address that is not banned.
func (l *banListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if addr, ok := remoteAddr(conn); ok && l.bans.IsBanned(addr) {
			conn.Close()
			continue
		}
		return conn, nil
	}
}
