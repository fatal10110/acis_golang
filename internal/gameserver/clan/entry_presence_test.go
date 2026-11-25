package clan

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// presenceFixture is clan 1 led by 10 with member 11, both rows stored at
// power grade 3, and a service writing to an in-memory store.
func presenceFixture(t *testing.T) (*Service, *Clan, *fakeStore) {
	t.Helper()
	now := time.UnixMilli(1_800_000_000_000)
	table := NewTable()
	table.Restore(Snapshot{
		Clans: []Row{{ID: 1, Name: "Home", Level: 5, LeaderID: 10}},
		Members: []MemberRow{
			{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lead", PowerGrade: 3}},
			{ClanID: 1, Member: Member{ObjectID: 11, Name: "Mate", PowerGrade: 3}},
		},
	}, now, 1)
	store := newFakeStore()
	store.grades[10], store.grades[11] = 3, 3
	cl, _ := table.Get(1)
	return NewService(table, store, nil, nil, DefaultConfig(), nil, zerolog.Nop()), cl, store
}

func removedMember(t *testing.T, out Dissolution, objectID int32) Member {
	t.Helper()
	for _, m := range out.Members {
		if m.ObjectID == objectID {
			return m
		}
	}
	t.Fatalf("the dissolution did not remove %d", objectID)
	return Member{}
}

// TestDissolutionRemovesMemberEnteredMeanwhileOnline enters member 11 into
// the world after the dissolution read the roster, while it resolves the
// leader: 11 leaves online, its online row written, as it is in the world.
func TestDissolutionRemovesMemberEnteredMeanwhileOnline(t *testing.T) {
	s, cl, store := presenceFixture(t)
	now := time.UnixMilli(1_800_000_000_000)
	lead, mate := inClan(10, "Lead", 1), inClan(11, "Mate", 1)
	cl.SetOnline(10, LiveMember(lead))
	s.BeginEntry(11)
	var entered bool
	online := func(id int32) *player.Character {
		switch id {
		case 10:
			if !entered {
				_, entered = s.Enter(mate)
			}
			return lead
		case 11:
			return mate
		}
		return nil
	}
	out, ok := s.Destroy(cl, false, online, now)
	if !ok || !entered {
		t.Fatalf("destroyed %v, entered %v", ok, entered)
	}
	m := removedMember(t, out, 11)
	if !m.Online || m.Entering {
		t.Fatalf("member 11 left with online %v entering %v, want online", m.Online, m.Entering)
	}
	if got := store.grades[11]; got != 0 {
		t.Fatalf("member 11 stored power grade %d, want the online row's 0", got)
	}
}

// TestDissolutionRemovesMemberLeftMeanwhileOffline takes member 11 out of
// the world while the dissolution resolves it: 11 leaves offline, its
// offline row written, its power grade kept.
func TestDissolutionRemovesMemberLeftMeanwhileOffline(t *testing.T) {
	s, cl, store := presenceFixture(t)
	now := time.UnixMilli(1_800_000_000_000)
	mate := inClan(11, "Mate", 1)
	cl.SetOnline(11, LiveMember(mate))
	left := false
	online := func(id int32) *player.Character {
		if id != 11 {
			return nil
		}
		if !left {
			left = true
			cl.SetOffline(11, LiveMember(mate))
		}
		return mate
	}
	out, ok := s.Destroy(cl, false, online, now)
	if !ok {
		t.Fatal("not destroyed")
	}
	m := removedMember(t, out, 11)
	if m.Online || m.Entering {
		t.Fatalf("member 11 left with online %v entering %v, want offline", m.Online, m.Entering)
	}
	if got := store.grades[11]; got != 3 {
		t.Fatalf("member 11 stored power grade %d, want the offline row's kept 3", got)
	}
}
