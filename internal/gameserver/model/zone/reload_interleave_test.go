package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ledgerStub is an occupant that drives the index itself, as a player's
// zone actor does: it revalidates with Revalidate, RevalidateMove and
// Rebase under its own lock instead of through a Member.
type ledgerStub struct {
	flags Flags
	id    int32
	class Class
	at    location.Location
}

func (s *ledgerStub) ObjectID() int32             { return s.id }
func (s *ledgerStub) Position() location.Location { return s.at }
func (s *ledgerStub) ZoneFlags() *Flags           { return &s.flags }
func (s *ledgerStub) Class() Class                { return s.class }

// TestReloadKeepsHoldsOfARevalidationBeforeTheRebase pins the window
// between a zone reload's Swap and the rebase of one creature: a creature
// whose move ends (Settle) or whose fifth step falls (Step) in that window
// leaves the old zones and enters the new ones then, and its later Reload
// keeps the holds the new zones raised. Occupancy and flags agree
// throughout, and leaving the new zone releases its hold.
func TestReloadKeepsHoldsOfARevalidationBeforeTheRebase(t *testing.T) {
	moves := map[string]func(ix *Index, s *memberStub){
		"settle": func(ix *Index, s *memberStub) { s.Settle(ix, s) },
		"step": func(ix *Index, s *memberStub) {
			for range StepsPerRevalidation {
				s.at.X++
				s.Step(ix, s)
			}
		},
	}
	for name, move := range moves {
		t.Run(name, func(t *testing.T) {
			ix := NewIndex()
			oldPeace := NewPeace(1, testCuboid(t, 0, 1000, 0, 1000))
			ix.Add(oldPeace)
			s := &memberStub{id: 1, class: ClassNPC, at: location.Location{X: 500, Y: 500}}
			s.Enter(ix, s)

			next := NewIndex()
			peace := NewPeace(2, testCuboid(t, 0, 1000, 0, 1000))
			next.Add(peace)
			old := ix.Swap(next)

			move(ix, s)
			if !peace.Inside(s) || !s.ZoneFlags().Has(FlagPeace) || oldPeace.Inside(s) {
				t.Fatal("a move after the swap did not leave the old peace zone for the new one")
			}
			s.Reload(ix, s)
			if !peace.Inside(s) || !s.ZoneFlags().Has(FlagPeace) {
				t.Fatalf("after Reload: inside the new peace zone = %v, peace flag = %v; want both",
					peace.Inside(s), s.ZoneFlags().Has(FlagPeace))
			}
			old.Clear()

			s.at = location.Location{X: 2000, Y: 500}
			s.Settle(ix, s)
			if peace.Inside(s) || s.ZoneFlags().Has(FlagPeace) {
				t.Fatal("leaving the new peace zone kept its hold or a hold of the old one")
			}
		})
	}
}

// TestReloadPlayerMoveBeforeTheRebase pins the same window on the path a
// player's zone actor takes: a RevalidateMove against the new zones before
// the player's Rebase drops the old arena without its exit rule and enters
// the new one, and the Rebase keeps the PvP flag the new arena raised.
func TestReloadPlayerMoveBeforeTheRebase(t *testing.T) {
	ix := NewIndex()
	oldArena := NewArena(1, testCuboid(t, 0, 1000, 0, 1000))
	var oldNotices int
	oldArena.CombatNotice = func(Actor, bool) { oldNotices++ }
	ix.Add(oldArena)
	p := &ledgerStub{id: 1, class: ClassPlayer, at: location.Location{X: 500, Y: 500}}
	ix.Revalidate(p)

	next := NewIndex()
	arena := NewArena(2, testCuboid(t, 0, 1000, 0, 1000))
	var entered int
	arena.CombatNotice = func(_ Actor, in bool) {
		if in {
			entered++
		}
	}
	next.Add(arena)
	old := ix.Swap(next)

	previous := p.at
	p.at = location.Location{X: 600, Y: 500}
	ix.RevalidateMove(p, previous)
	ix.Rebase(p, p.at, true)
	if !arena.Inside(p) || !p.flags.Has(FlagPvP) || !p.flags.Has(FlagNoSummonFriend) {
		t.Fatalf("after Rebase: inside the new arena = %v, pvp flag = %v; want both",
			arena.Inside(p), p.flags.Has(FlagPvP))
	}
	if oldArena.Inside(p) || oldNotices != 1 || entered != 1 {
		t.Fatalf("old arena holds the player = %v, old notices = %d (want only the first entry), new entries = %d (want 1)",
			oldArena.Inside(p), oldNotices, entered)
	}
	old.Clear()

	p.at = location.Location{X: 2000, Y: 500}
	ix.Revalidate(p)
	if p.flags.Has(FlagPvP) || p.flags.Has(FlagNoSummonFriend) {
		t.Fatal("leaving the new arena kept a PvP hold")
	}
}

// TestReloadCreatureTheRebaseMissed pins a creature the reload's walk did
// not reach: Clear empties the old zones, and the creature's next
// revalidation drops the old zones' flags, tells it it left the old water
// and enters the new zones; one that leaves the grid first only drops them.
func TestReloadCreatureTheRebaseMissed(t *testing.T) {
	ix := NewIndex()
	ix.Add(NewWater(1, testCuboid(t, 0, 1000, 0, 1000)))
	ix.Add(NewNoStore(2, testCuboid(t, 0, 1000, 0, 1000)))
	swimmer := &swimmingStub{memberStub: memberStub{id: 1, class: ClassNPC, at: location.Location{X: 500, Y: 500}}}
	swimmer.Enter(ix, swimmer)
	leaver := &memberStub{id: 2, class: ClassNPC, at: location.Location{X: 500, Y: 500}}
	leaver.Enter(ix, leaver)

	next := NewIndex()
	peace := NewPeace(3, testCuboid(t, 0, 1000, 0, 1000))
	next.Add(peace)
	ix.Swap(next).Clear()

	swimmer.Settle(ix, swimmer)
	flags := swimmer.ZoneFlags()
	if flags.Has(FlagWater) || flags.Has(FlagNoStore) || !flags.Has(FlagPeace) || !peace.Inside(swimmer) {
		t.Fatal("the missed creature kept the old zones' flags or did not enter the new zone")
	}
	if len(swimmer.swims) != 2 || swimmer.swims[1] {
		t.Fatalf("swim changes = %v, want entering then leaving the water", swimmer.swims)
	}

	leaver.Leave(ix, leaver)
	if leaver.ZoneFlags().Has(FlagNoStore) || peace.Inside(leaver) {
		t.Fatal("leaving the grid after the swap kept the old zones' flags or entered the new zone")
	}
}
