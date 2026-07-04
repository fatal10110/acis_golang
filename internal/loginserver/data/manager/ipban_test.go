package manager

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	l := LoadIPBanList(path, nil)

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
	l := LoadIPBanList(filepath.Join(t.TempDir(), "does-not-exist.properties"), nil)
	if got := len(l.bans); got != 0 {
		t.Fatalf("loaded %d bans from missing file, want 0", got)
	}
	if l.IsBanned(net.ParseIP("1.2.3.4")) {
		t.Error("nothing should be banned")
	}
}

func TestIPBanList_BanPermanent(t *testing.T) {
	l := NewIPBanList(nil)
	addr := net.ParseIP("10.0.0.1")

	l.Ban(addr, 0)
	if !l.IsBanned(addr) {
		t.Fatal("expected permanent ban to be active")
	}
}

func TestIPBanList_BanExpires(t *testing.T) {
	l := NewIPBanList(nil)
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
	l := NewIPBanList(nil)
	addr := net.ParseIP("10.0.0.3")

	l.Ban(addr, 0)           // permanent first
	l.Ban(addr, time.Second) // second call must not overwrite

	if until := l.bans[addr.String()]; !until.IsZero() {
		t.Fatalf("second Ban call overwrote existing entry: got %v, want permanent", until)
	}
}

func TestIPBanList_IsBanned_NilAddress(t *testing.T) {
	l := NewIPBanList(nil)
	if !l.IsBanned(nil) {
		t.Fatal("nil address should be treated as banned")
	}
}
