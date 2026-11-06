package netutil

import (
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"
)

// banClock is a settable clock for BanList.
type banClock struct{ at time.Time }

func (c *banClock) now() time.Time { return c.at }

func TestBanListAddRemoveAndExpiry(t *testing.T) {
	clock := &banClock{at: time.Unix(1_000_000, 0)}
	b := NewBanList(clock.now)
	perm := netip.MustParseAddr("203.0.113.1")
	temp := netip.MustParseAddr("203.0.113.2")

	if !b.Add(perm, 0) || !b.Add(temp, time.Minute) {
		t.Fatal("Add of a new address reported no ban")
	}
	// An address already banned keeps its ban and expiry.
	if b.Add(temp, time.Hour) {
		t.Fatal("Add over a ban in force replaced it")
	}
	// The IPv4-mapped form is the same host.
	if !b.IsBanned(netip.MustParseAddr("::ffff:203.0.113.1")) {
		t.Fatal("IPv4-mapped form of a banned address not banned")
	}

	want := []BanEntry{{Addr: perm}, {Addr: temp, Expires: clock.at.Add(time.Minute)}}
	got := b.List()
	slices.SortFunc(got, func(a, b BanEntry) int { return a.Addr.Compare(b.Addr) })
	if !slices.Equal(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}

	// A ban holds through its expiry instant and lifts after it.
	clock.at = clock.at.Add(time.Minute)
	if !b.IsBanned(temp) {
		t.Fatal("ban lifted at its expiry instant")
	}
	clock.at = clock.at.Add(time.Nanosecond)
	if b.IsBanned(temp) || b.Remove(temp) {
		t.Fatal("expired ban still in force")
	}
	if got := b.List(); len(got) != 1 || got[0].Addr != perm || !got[0].Permanent() {
		t.Fatalf("List after expiry = %v, want only the permanent ban", got)
	}
	// An expired address takes a new ban.
	if !b.Add(temp, time.Second) {
		t.Fatal("Add after expiry reported no ban")
	}

	if !b.Remove(perm) || b.IsBanned(perm) || b.Remove(perm) {
		t.Fatal("Remove did not lift the permanent ban exactly once")
	}
}

func TestRefuseBannedClosesBannedConnections(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := NewBanList(time.Now)
	b.Add(netip.MustParseAddr("127.0.0.2"), 0)
	ln := RefuseBanned(inner, b)
	t.Cleanup(func() { ln.Close() })

	accepted := make(chan net.Conn, 2)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				close(accepted)
				return
			}
			accepted <- c
		}
	}()

	dial := func(local string) net.Conn {
		t.Helper()
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(local)}}
		c, err := d.Dial("tcp", inner.Addr().String())
		if err != nil {
			t.Fatalf("dial from %s: %v", local, err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}

	banned := dial("127.0.0.2")
	_ = banned.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := banned.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Fatalf("banned connection read n=%d err=%v, want a close", n, err)
	}

	dial("127.0.0.3")
	c := <-accepted
	if got := c.RemoteAddr().(*net.TCPAddr).IP.String(); got != "127.0.0.3" {
		t.Fatalf("accepted %s, want only 127.0.0.3", got)
	}
	c.Close()
}
