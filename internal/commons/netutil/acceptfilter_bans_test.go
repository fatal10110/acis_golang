package netutil

import (
	"net/netip"
	"testing"
	"time"
)

// List shows only banned addresses, each expiring BanTime after its last
// attempt; Remove lifts a ban and resets the address's count, and leaves an
// address that is only counting tries alone.
func TestAcceptFilterListAndRemove(t *testing.T) {
	f := defaultFilter()
	banned := netip.MustParseAddr("203.0.113.1")
	counting := netip.MustParseAddr("203.0.113.2")
	at := time.Unix(1_000_000, 0)

	for i := range 5 {
		at = at.Add(100 * time.Millisecond)
		if got, want := f.Allow(banned, at), i < 4; got != want {
			t.Fatalf("connection %d allowed=%v, want %v", i+1, got, want)
		}
	}
	f.Allow(counting, at)
	f.Allow(counting, at.Add(time.Millisecond))

	want := []BanEntry{{Addr: banned, Expires: at.Add(defaultBanTime)}}
	if got := f.List(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("List = %v, want %v", got, want)
	}
	// Another refused attempt pushes the expiry back.
	at = at.Add(time.Second)
	f.Allow(banned, at)
	if got := f.List(); len(got) != 1 || !got[0].Expires.Equal(at.Add(defaultBanTime)) {
		t.Fatalf("List after a retry = %v, want expiry %v", got, at.Add(defaultBanTime))
	}

	if f.Remove(counting) {
		t.Fatal("Remove reported a ban on an address only counting tries")
	}
	if !f.Remove(banned) || f.Remove(banned) {
		t.Fatal("Remove did not lift the ban exactly once")
	}
	if got := f.List(); len(got) != 0 {
		t.Fatalf("List after Remove = %v, want none", got)
	}
	// The record went with the ban: four burst connections pass again.
	for i := range 4 {
		at = at.Add(100 * time.Millisecond)
		if !f.Allow(banned, at) {
			t.Fatalf("connection %d after Remove refused", i+1)
		}
	}
}
