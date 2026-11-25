package clan

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestEntryTakesRemovalInFlight removes members offline while their
// selection is in flight and checks the character the selection loaded
// takes the removal's columns, at RestoreMembership, at Enter, or when the
// selection ends without entering; a member not in flight, or removed
// online, keeps nothing.
func TestEntryTakesRemovalInFlight(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	join := now.UnixMilli() + dayMillis
	table := NewTable()
	var members []MemberRow
	for id := int32(10); id <= 15; id++ {
		members = append(members, MemberRow{ClanID: 1, Member: Member{ObjectID: id, Name: "M" + strconv.Itoa(int(id))}})
	}
	table.Restore(Snapshot{Clans: []Row{{ID: 1, Name: "Home", Level: 5, LeaderID: 10}}, Members: members}, now, 1)
	s := NewService(table, nil, nil, nil, DefaultConfig(), nil, zerolog.Nop())
	cl, _ := table.Get(1)

	settled := func(t *testing.T, id int32, settle func(c *player.Character)) {
		t.Helper()
		c := inClan(id, "M"+strconv.Itoa(int(id)), 1)
		c.SetTitle("Lord")
		c.SetWantsPeace(true)
		settle(c)
		if c.ClanID() != 0 || c.Title() != "" || c.ClanJoinExpiryTime() != join || c.WantsPeace() {
			t.Fatalf("member %d: clan %d title %q join %d peace %v, want the removal's columns",
				id, c.ClanID(), c.Title(), c.ClanJoinExpiryTime(), c.WantsPeace())
		}
	}

	// Removed before its clan state is read: RestoreMembership applies it.
	s.BeginEntry(11)
	if m, _ := s.remove(cl, 11, join, nil, now); !m.Entering {
		t.Fatal("member 11 in flight removed offline not marked Entering")
	}
	settled(t, 11, func(c *player.Character) { s.RestoreMembership(c, now) })

	// Removed after its clan state was read: Enter applies it.
	s.BeginEntry(12)
	c12 := inClan(12, "M12", 1)
	s.RestoreMembership(c12, now)
	if c12.ClanID() != 1 {
		t.Fatal("member 12 lost its clan before its removal")
	}
	s.remove(cl, 12, join, nil, now)
	settled(t, 12, func(c *player.Character) {
		if _, ok := s.Enter(c); ok {
			t.Fatal("Enter found member 12 in the clan that removed it")
		}
	})

	// The selection ends without entering: EndEntry applies it.
	s.BeginEntry(13)
	s.remove(cl, 13, join, nil, now)
	settled(t, 13, func(c *player.Character) { s.EndEntry(c) })

	// Not in flight, or removed online: nothing is kept.
	if m, _ := s.remove(cl, 14, join, nil, now); m.Entering {
		t.Fatal("member 14, not in flight, marked Entering")
	}
	s.BeginEntry(15)
	if m, _ := s.remove(cl, 15, join, inClan(15, "M15", 1), now); m.Entering {
		t.Fatal("member 15, removed online, marked Entering")
	}
	if _, ok := s.entries.take(15, true); ok {
		t.Fatal("a removal online was kept for member 15")
	}

	// Still a member: Enter marks it online and ends its flight.
	s.BeginEntry(10)
	lead := inClan(10, "M10", 1)
	if got, ok := s.Enter(lead); !ok || got != cl {
		t.Fatal("Enter did not return the leader's clan")
	}
	if m, _ := cl.Member(10); !m.Online {
		t.Fatal("Enter left the leader offline")
	}
	if s.entries.depart(RemovalRow{ObjectID: 10}) {
		t.Fatal("the leader is still in flight after Enter")
	}
}

// TestEntryRemovalRacesEnter removes a member while it enters, many times:
// it either enters as a member or takes the removal, never neither.
func TestEntryRemovalRacesEnter(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	join := now.UnixMilli() + dayMillis
	for i := range 200 {
		table := NewTable()
		table.Restore(Snapshot{
			Clans:   []Row{{ID: 1, Name: "Home", Level: 5, LeaderID: 10}},
			Members: []MemberRow{{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lead"}}, {ClanID: 1, Member: Member{ObjectID: 11, Name: "Mate"}}},
		}, now, 1)
		s := NewService(table, nil, nil, nil, DefaultConfig(), nil, zerolog.Nop())
		cl, _ := table.Get(1)
		s.BeginEntry(11)
		c := inClan(11, "Mate", 1)
		var removed Member
		var wg sync.WaitGroup
		wg.Go(func() {
			// A member already online is removed online, as the expulsion
			// finds it in the world.
			removed, _ = s.removeMember(cl, 11, join, func(int32) *player.Character { return c }, now)
		})
		_, entered := s.Enter(c)
		wg.Wait()
		switch {
		case entered && (!removed.Online || removed.Entering):
			t.Fatalf("run %d: entered as a member but removed offline (online %v, entering %v)", i, removed.Online, removed.Entering)
		case !entered && (c.ClanID() != 0 || c.ClanJoinExpiryTime() != join):
			t.Fatalf("run %d: removed first but entered with clan %d join %d", i, c.ClanID(), c.ClanJoinExpiryTime())
		}
	}
}

// TestEntryDissolutionRacesEnter dissolves a clan while its leader enters,
// many times: the leader either enters as a member and is dissolved online,
// or takes the removal's columns, with no title and the creation penalty a
// leader leaving takes.
func TestEntryDissolutionRacesEnter(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	create := now.UnixMilli() + int64(DefaultConfig().CreateDays)*dayMillis
	for i := range 500 {
		table := NewTable()
		table.Restore(Snapshot{
			Clans:   []Row{{ID: 1, Name: "Home", Level: 5, LeaderID: 10}},
			Members: []MemberRow{{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lead"}}, {ClanID: 1, Member: Member{ObjectID: 11, Name: "Mate"}}},
		}, now, 1)
		s := NewService(table, nil, nil, nil, DefaultConfig(), nil, zerolog.Nop())
		cl, _ := table.Get(1)
		s.BeginEntry(10)
		c := inClan(10, "Lead", 1)
		c.SetTitle("Lord")
		var out Dissolution
		var wg sync.WaitGroup
		wg.Go(func() {
			out, _ = s.Destroy(cl, false, func(int32) *player.Character { return c }, now)
		})
		_, entered := s.Enter(c)
		wg.Wait()
		var lead Member
		for _, m := range out.Members {
			if m.ObjectID == 10 {
				lead = m
			}
		}
		switch {
		case lead.ObjectID == 0:
			t.Fatalf("run %d: the dissolution did not remove the leader", i)
		case entered && (!lead.Online || lead.Entering):
			t.Fatalf("run %d: entered as a member but dissolved offline (online %v, entering %v)", i, lead.Online, lead.Entering)
		case !entered && (!lead.Entering || c.ClanID() != 0 || c.Title() != "" || c.ClanCreateExpiryTime() != create):
			t.Fatalf("run %d: entered clanless with entering %v clan %d title %q create %d, want the removal's columns",
				i, lead.Entering, c.ClanID(), c.Title(), c.ClanCreateExpiryTime())
		}
	}
}
