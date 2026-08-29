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
