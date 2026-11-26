package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestDissolutionHandsRemovalToMemberLeavingWorld dissolves the clan while
// member 11, listed online, is already out of the world on its way out:
// no row is written for it then; once it is marked offline (Leave), it
// writes the online row, taking its own creation penalty, and takes the
// leave.
func TestDissolutionHandsRemovalToMemberLeavingWorld(t *testing.T) {
	s, cl, store := presenceFixture(t)
	now := time.UnixMilli(1_800_000_000_000)
	mate := inClan(11, "Mate", 1)
	mate.SetClanCreateExpiryTime(now.UnixMilli() + 5000)
	cl.SetOnline(11, LiveMember(mate))
	store.clanIDs[11] = 1
	out, ok := s.Destroy(cl, false, func(int32) *player.Character { return nil }, now)
	if !ok {
		t.Fatal("not destroyed")
	}
	if m := removedMember(t, out, 11); !m.Online || m.Entering {
		t.Fatalf("member 11 left with online %v entering %v, want online", m.Online, m.Entering)
	}
	if got := store.clanIDs[11]; got != 1 {
		t.Fatalf("member 11 stored clan %d before it was marked offline, want its row untouched", got)
	}

	if _, ok := s.Leave(mate); ok {
		t.Fatal("Leave found member 11 still in its dissolved clan")
	}
	if got := store.grades[11]; got != 0 {
		t.Fatalf("member 11 stored power grade %d, want the online row's 0", got)
	}
	if got := store.clanIDs[11]; got != 0 {
		t.Fatalf("member 11 stored clan %d, want 0", got)
	}
	if mate.ClanID() != 0 || mate.ClanCreateExpiryTime() != now.UnixMilli()+5000 {
		t.Fatalf("member 11 left with clan %d, creation penalty %d", mate.ClanID(), mate.ClanCreateExpiryTime())
	}
	if _, ok := s.Leave(mate); ok {
		t.Fatal("a second Leave found a clan")
	}
}
