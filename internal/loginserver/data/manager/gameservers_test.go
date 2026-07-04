package manager

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/loginserver/link"
)

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

	entry, ok := r.MarkOnline(5, "1.2.3.4", 7777, 100)
	if !ok {
		t.Fatal("MarkOnline() = false, want true")
	}
	if !entry.Authed || entry.Host != "1.2.3.4" || entry.Port != 7777 || entry.MaxPlayers != 100 {
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
