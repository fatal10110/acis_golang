package manager

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Add, Remove and List, the ban API's view of the list: Add keeps a ban in
// force but replaces an expired one, Remove reports only a ban in force,
// and List leaves out expired bans without dropping them.
func TestIPBanListAddRemoveList(t *testing.T) {
	at := time.Unix(1_000_000, 0)
	l := NewIPBanList(zerolog.Nop())
	l.now = func() time.Time { return at }
	temp := netip.MustParseAddr("203.0.113.1")
	perm := netip.MustParseAddr("::ffff:203.0.113.2") // the IPv4 host

	if !l.Add(temp, time.Minute) || !l.Add(perm, 0) {
		t.Fatal("Add of a new address reported no ban")
	}
	if l.Add(temp, time.Hour) || l.Ban(net.ParseIP("203.0.113.1"), time.Hour) {
		t.Fatal("a second ban replaced one in force")
	}
	if !l.IsBanned(net.ParseIP("203.0.113.2")) {
		t.Fatal("IPv4-mapped Add did not ban the IPv4 host")
	}
	if got := l.List(); len(got) != 2 {
		t.Fatalf("List = %v, want two bans", got)
	}

	at = at.Add(time.Minute + time.Second)
	got := l.List()
	if len(got) != 1 || got[0].Addr != netip.MustParseAddr("203.0.113.2") || !got[0].Permanent() {
		t.Fatalf("List after expiry = %v, want only the permanent ban", got)
	}
	// The expired entry is still held: Ban keeps it, Add replaces it.
	if l.Ban(net.ParseIP("203.0.113.1"), time.Minute) {
		t.Fatal("Ban replaced an expired entry")
	}
	if !l.Add(temp, time.Minute) {
		t.Fatal("Add did not replace an expired entry")
	}
	if !l.Remove(temp) || l.Remove(temp) || l.IsBanned(net.ParseIP("203.0.113.1")) {
		t.Fatal("Remove did not lift the ban exactly once")
	}

	l.Add(temp, time.Minute)
	at = at.Add(2 * time.Minute)
	if l.Remove(temp) {
		t.Fatal("Remove reported an expired ban as in force")
	}
	if l.Ban(net.ParseIP("203.0.113.1"), time.Minute) != true {
		t.Fatal("Remove left the expired entry behind")
	}
}
