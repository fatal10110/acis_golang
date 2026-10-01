package manager

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
)

// ---- from gameservers_test.go ----
func TestServerRegistryRegisterRejectsDuplicateID(t *testing.T) {
	r := NewServerRegistry()
	if _, ok := r.Register(1, []byte{0x01}); !ok {
		t.Fatal("first Register() = false, want true")
	}
	if _, ok := r.Register(1, []byte{0x02}); ok {
		t.Fatal("second Register() with same id = true, want false")
	}
}

func TestServerRegistryRegisterFirstSkipsTaken(t *testing.T) {
	r := NewServerRegistry()
	if _, ok := r.Register(1, []byte{0xaa}); !ok {
		t.Fatal("Register(1) = false")
	}

	entry, ok := r.RegisterFirst([]int{1, 2, 3}, []byte{0xbb})
	if !ok {
		t.Fatal("RegisterFirst() = false, want true")
	}
	if entry.ID != 2 {
		t.Fatalf("RegisterFirst() id = %d, want 2", entry.ID)
	}
}

func TestServerRegistryRegisterFirstFailsWhenFull(t *testing.T) {
	r := NewServerRegistry()
	r.Register(1, nil)
	r.Register(2, nil)
	if _, ok := r.RegisterFirst([]int{1, 2}, []byte{0x01}); ok {
		t.Fatal("RegisterFirst() = true, want false when every candidate id is taken")
	}
}

func TestServerRegistryMarkOnlineOffline(t *testing.T) {
	r := NewServerRegistry()
	r.Register(5, []byte{0x01})

	entry, ok := r.MarkOnline(5, "1.2.3.4", net.ParseIP("203.0.113.9"), 7777, 100)
	if !ok {
		t.Fatal("MarkOnline() = false, want true")
	}
	if !entry.Authed || entry.Host != "1.2.3.4" || !entry.ConnIP.Equal(net.ParseIP("203.0.113.9")) || entry.Port != 7777 || entry.MaxPlayers != 100 {
		t.Fatalf("MarkOnline() entry = %+v", entry)
	}

	r.AddOnlineAccount(5, "acc1")
	if got := r.OnlineAccountCount(5); got != 1 {
		t.Fatalf("OnlineAccountCount() = %d, want 1", got)
	}

	r.MarkOffline(5)
	entry, _ = r.Get(5)
	if entry.Authed || entry.Port != 0 || entry.Status != link.ServerTypeDown {
		t.Fatalf("after MarkOffline() entry = %+v", entry)
	}
	if got := r.OnlineAccountCount(5); got != 0 {
		t.Fatalf("OnlineAccountCount() after MarkOffline() = %d, want 0", got)
	}
}

func TestServerRegistryMarkOnlineRejectsAlreadyAuthedServer(t *testing.T) {
	r := NewServerRegistry()
	r.Register(5, []byte{0x01})
	if _, ok := r.MarkOnline(5, "1.2.3.4", net.ParseIP("203.0.113.9"), 7777, 100); !ok {
		t.Fatal("first MarkOnline() = false, want true")
	}
	if _, ok := r.MarkOnline(5, "5.6.7.8", net.ParseIP("203.0.113.10"), 7778, 200); ok {
		t.Fatal("second MarkOnline() = true, want false for already-authed server")
	}

	entry, _ := r.Get(5)
	if entry.Host != "1.2.3.4" || entry.Port != 7777 || entry.MaxPlayers != 100 {
		t.Fatalf("entry after rejected MarkOnline() = %+v", entry)
	}
}

func TestServerRegistryApplyStatusLeavesUnsetFieldsUnchanged(t *testing.T) {
	r := NewServerRegistry()
	r.Register(1, nil)

	good := link.ServerTypeGood
	on := true
	r.ApplyStatus(1, link.ServerStatus{Status: &good, ShowClock: &on})

	full := link.ServerTypeFull
	age := int32(18)
	entry, ok := r.ApplyStatus(1, link.ServerStatus{Status: &full, AgeLimit: &age})
	if !ok {
		t.Fatal("ApplyStatus() = false, want true")
	}
	if entry.Status != link.ServerTypeFull || entry.AgeLimit != 18 || !entry.ShowClock {
		t.Fatalf("ApplyStatus() entry = %+v, want Status=Full AgeLimit=18 ShowClock=true (untouched)", entry)
	}
}

func TestServerRegistryLoadSeedsOfflineEntries(t *testing.T) {
	r := NewServerRegistry()
	r.Load(map[int][]byte{1: {0xde, 0xad}})

	entry, ok := r.Get(1)
	if !ok {
		t.Fatal("Get(1) after Load() = false, want true")
	}
	if entry.Authed {
		t.Fatal("loaded entry.Authed = true, want false")
	}
	if !bytes.Equal(entry.HexID, []byte{0xde, 0xad}) {
		t.Fatalf("loaded entry.HexID = %x, want dead", entry.HexID)
	}
}

// ---- from ipban_test.go ----
func writeBanFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "banned_ips.properties")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadIPBanList_SkipsCommentsAndBadLines(t *testing.T) {
	path := writeBanFile(t, "# comment line\n1.2.3.4\nnot-an-ip-and-no-dns\n::1\n")
	l := LoadIPBanList(path, zerolog.Nop())

	if got := len(l.bans); got != 2 {
		t.Fatalf("loaded %d bans, want 2", got)
	}
	if !l.IsBanned(net.ParseIP("1.2.3.4")) {
		t.Error("1.2.3.4 should be banned")
	}
	if !l.IsBanned(net.ParseIP("::1")) {
		t.Error("::1 should be banned")
	}
}

func TestLoadIPBanList_MissingFile(t *testing.T) {
	l := LoadIPBanList(filepath.Join(t.TempDir(), "does-not-exist.properties"), zerolog.Nop())
	if got := len(l.bans); got != 0 {
		t.Fatalf("loaded %d bans from missing file, want 0", got)
	}
	if l.IsBanned(net.ParseIP("1.2.3.4")) {
		t.Error("nothing should be banned")
	}
}

func TestIPBanList_BanPermanent(t *testing.T) {
	l := NewIPBanList(zerolog.Nop())
	addr := net.ParseIP("10.0.0.1")

	l.Ban(addr, 0)
	if !l.IsBanned(addr) {
		t.Fatal("expected permanent ban to be active")
	}
}

func TestIPBanList_BanExpires(t *testing.T) {
	l := NewIPBanList(zerolog.Nop())
	addr := net.ParseIP("10.0.0.2")

	l.Ban(addr, 10*time.Millisecond)
	if !l.IsBanned(addr) {
		t.Fatal("expected ban to be active immediately")
	}

	time.Sleep(50 * time.Millisecond)
	if l.IsBanned(addr) {
		t.Fatal("expected ban to have expired")
	}
	if _, stillPresent := l.bans[addr.String()]; stillPresent {
		t.Fatal("expired ban should be removed from the map")
	}
}

func TestIPBanList_BanKeepsExistingExpiry(t *testing.T) {
	l := NewIPBanList(zerolog.Nop())
	addr := net.ParseIP("10.0.0.3")

	l.Ban(addr, 0)           // permanent first
	l.Ban(addr, time.Second) // second call must not overwrite

	if until := l.bans[addr.String()]; !until.IsZero() {
		t.Fatalf("second Ban call overwrote existing entry: got %v, want permanent", until)
	}
}

func TestIPBanList_IsBanned_NilAddress(t *testing.T) {
	l := NewIPBanList(zerolog.Nop())
	if !l.IsBanned(nil) {
		t.Fatal("nil address should be treated as banned")
	}
}

// clockedIPBanList returns an empty list whose Ban and IsBanned read *now.
func clockedIPBanList(now *time.Time) *IPBanList {
	l := NewIPBanList(zerolog.Nop())
	l.now = func() time.Time { return *now }
	return l
}

func retainNone(string) bool { return false }

// Expected values follow the reference ban list: a zero expiry never lifts,
// and a timed ban lifts only once the current time is strictly past its
// expiry (IpBanManager.isBannedAddress, "time > 0 && time < now").
func TestIPBanList_SweepExpiredDropsOnlyExpiredTemporaryBans(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	l := clockedIPBanList(&now)

	permanent := net.ParseIP("192.0.2.1")
	l.Ban(permanent, 0)
	const temporary = 2000
	for i := range temporary {
		l.Ban(net.IPv4(10, 2, byte(i>>8), byte(i)), 10*time.Minute)
	}
	now = start.Add(time.Minute)
	later := net.ParseIP("192.0.2.2")
	l.Ban(later, 10*time.Minute)

	expiry := start.Add(10 * time.Minute)
	l.SweepExpired(expiry, retainNone)
	if got, want := len(l.bans), temporary+2; got != want {
		t.Fatalf("after sweeping at the expiry instant: %d bans, want %d (a ban is still active at its expiry)", got, want)
	}

	l.SweepExpired(expiry.Add(time.Nanosecond), retainNone)
	if got := len(l.bans); got != 2 {
		t.Fatalf("after sweeping past expiry: %d bans, want 2 (permanent + unexpired)", got)
	}
	now = expiry.Add(time.Nanosecond)
	if !l.IsBanned(permanent) || !l.IsBanned(later) {
		t.Fatal("sweep removed a ban that has not expired")
	}

	l.SweepExpired(start.Add(100*365*24*time.Hour), retainNone)
	if got := len(l.bans); got != 1 {
		t.Fatalf("after sweeping far in the future: %d bans, want 1", got)
	}
	if !l.IsBanned(permanent) {
		t.Fatal("sweep removed a permanent ban")
	}
}

// A swept list answers every ban check exactly as an unswept one.
func TestIPBanList_SweepExpiredDoesNotChangeBanChecks(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nowSwept, nowPlain := start, start
	swept, plain := clockedIPBanList(&nowSwept), clockedIPBanList(&nowPlain)

	addrs := []net.IP{net.ParseIP("192.0.2.10"), net.ParseIP("192.0.2.11"), net.ParseIP("192.0.2.12"), net.ParseIP("2001:db8::1")}
	durations := []time.Duration{0, time.Second, 10 * time.Minute, time.Hour}
	for i, addr := range addrs {
		swept.Ban(addr, durations[i])
		plain.Ban(addr, durations[i])
	}

	for _, at := range []time.Duration{0, time.Second, time.Second + time.Nanosecond, 10 * time.Minute, 10*time.Minute + time.Millisecond, 2 * time.Hour} {
		nowSwept, nowPlain = start.Add(at), start.Add(at)
		swept.SweepExpired(nowSwept, retainNone)
		for _, addr := range addrs {
			if got, want := swept.IsBanned(addr), plain.IsBanned(addr); got != want {
				t.Fatalf("at +%v IsBanned(%v) = %v after sweeping, %v without", at, addr, got, want)
			}
		}
	}
}

// While its address may still have an open connection, an expired ban keeps
// deciding Ban's outcome: Ban leaves an existing entry alone, so a new ban
// on top of the expired one has no effect (IpBanManager.addBanForAddress
// putIfAbsent) and the next check lifts it. Unretained, the entry is swept
// and a later connect-then-ban bans as it would have anyway.
func TestIPBanList_SweepExpiredLeavesRetainedAddresses(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	l := clockedIPBanList(&now)

	held, idle := net.ParseIP("192.0.2.20"), net.ParseIP("192.0.2.21")
	l.Ban(held, time.Minute)
	l.Ban(idle, time.Minute)

	now = start.Add(time.Hour)
	l.SweepExpired(now, func(addr string) bool { return addr == held.String() })
	if _, ok := l.bans[held.String()]; !ok {
		t.Fatal("sweep removed an expired ban whose address is retained")
	}
	if _, ok := l.bans[idle.String()]; ok {
		t.Fatal("sweep kept an expired ban whose address is not retained")
	}

	l.Ban(held, 10*time.Minute)
	if l.IsBanned(held) {
		t.Fatal("ban over a retained expired entry took effect; it must keep the expired entry")
	}
	if _, ok := l.bans[held.String()]; ok {
		t.Fatal("IsBanned did not lift the expired entry")
	}

	if l.IsBanned(idle) {
		t.Fatal("swept address reported banned on connect")
	}
	l.Ban(idle, 10*time.Minute)
	if !l.IsBanned(idle) {
		t.Fatal("ban after the connect check did not take effect")
	}
}

// ---- from rsapool_test.go ----
func TestNewRSAKeyPool(t *testing.T) {
	pool, err := NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}
	if len(pool.keys) != gsKeyPoolSize {
		t.Fatalf("len(pool.keys) = %d, want %d", len(pool.keys), gsKeyPoolSize)
	}
	for i, k := range pool.keys {
		if got := k.N.BitLen(); got != gsKeyBits {
			t.Fatalf("pool.keys[%d] bit length = %d, want %d", i, got, gsKeyBits)
		}
	}

	for i := 0; i < 50; i++ {
		k := pool.Random()
		if k == nil {
			t.Fatal("Random() returned nil")
		}
	}
}

// ---- from sessions_test.go ----
func TestSessionStorePutGetDelete(t *testing.T) {
	s := NewSessionStore()

	if _, ok := s.Get("acc1"); ok {
		t.Fatal("Get() on empty store = true, want false")
	}

	key := link.SessionKey{PlayKey1: 1, PlayKey2: 2, LoginKey1: 3, LoginKey2: 4}
	s.Put("acc1", key)

	got, ok := s.Get("acc1")
	if !ok || got != key {
		t.Fatalf("Get() = %+v, %v, want %+v, true", got, ok, key)
	}

	s.Delete("acc1")
	if _, ok := s.Get("acc1"); ok {
		t.Fatal("Get() after Delete() = true, want false")
	}
}
