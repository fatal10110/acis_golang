package zone

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type fakeActor struct {
	id    int32
	pos   location.Location
	flags Flags
	class Class
}

func (a *fakeActor) ObjectID() int32             { return a.id }
func (a *fakeActor) Position() location.Location { return a.pos }
func (a *fakeActor) ZoneFlags() *Flags           { return &a.flags }
func (a *fakeActor) Class() Class                { return a.class }

type fakePlayer struct {
	fakeActor
	gm     bool
	online bool
	race   player.Race
	clan   int32
}

func newFakePlayer(id int32, pos location.Location) *fakePlayer {
	return &fakePlayer{fakeActor: fakeActor{id: id, pos: pos, class: ClassPlayer}, online: true}
}

func (p *fakePlayer) GM() bool          { return p.gm }
func (p *fakePlayer) Online() bool      { return p.online }
func (p *fakePlayer) Race() player.Race { return p.race }
func (p *fakePlayer) ClanID() int32     { return p.clan }

type fakeSummon struct {
	fakeActor
	owner *fakePlayer
}

func newFakeSummon(id int32, pos location.Location, owner *fakePlayer) *fakeSummon {
	return &fakeSummon{fakeActor: fakeActor{id: id, pos: pos, class: ClassSummon}, owner: owner}
}

func (s *fakeSummon) Owner() (Player, bool) { return s.owner, s.owner != nil }

var (
	testForm = NewCuboid(0, 1000, 0, 1000, -100, 100)
	insideAt = location.Location{X: 500, Y: 500, Z: 0}
	outside  = location.Location{X: 5000, Y: 5000, Z: 0}
)

func TestRevalidateEnterOnceAndExit(t *testing.T) {
	z := NewPeace(1, testForm)
	a := newFakePlayer(7, insideAt)

	Revalidate(z, a)
	if !z.Inside(a) {
		t.Fatal("actor inside the bounds was not admitted")
	}
	if !a.flags.Has(FlagPeace) {
		t.Fatal("peace flag not raised on entry")
	}

	// Second revalidation in place must not stack the flag again.
	Revalidate(z, a)
	a.flags.Set(FlagPeace, false)
	if a.flags.Has(FlagPeace) {
		t.Fatal("peace flag was raised twice for a single entry")
	}
	a.flags.Set(FlagPeace, true) // restore the single hold

	a.pos = outside
	Revalidate(z, a)
	if z.Inside(a) {
		t.Fatal("actor outside the bounds is still tracked")
	}
	if a.flags.Has(FlagPeace) {
		t.Fatal("peace flag not released on exit")
	}
}

func TestRemoveWithoutEntryIsNoOp(t *testing.T) {
	z := NewPeace(1, testForm)
	a := newFakePlayer(7, insideAt)
	Remove(z, a)
	if a.flags.Has(FlagPeace) {
		t.Fatal("exit rules ran for an actor that never entered")
	}
}

func TestWatchersFireBeforeKindRules(t *testing.T) {
	z := NewPeace(1, testForm)
	a := newFakePlayer(7, insideAt)

	var sawPeaceOnEnter, sawPeaceOnExit bool
	z.OnEnter(func(w Actor) { sawPeaceOnEnter = w.ZoneFlags().Has(FlagPeace) })
	z.OnExit(func(w Actor) { sawPeaceOnExit = w.ZoneFlags().Has(FlagPeace) })

	Revalidate(z, a)
	if sawPeaceOnEnter {
		t.Error("enter watcher ran after the zone's entry rules, want before")
	}
	a.pos = outside
	Revalidate(z, a)
	if !sawPeaceOnExit {
		t.Error("exit watcher ran after the zone's exit rules, want before")
	}
}

func TestOverlappingZonesStackFlags(t *testing.T) {
	z1 := NewPeace(1, testForm)
	z2 := NewPeace(2, testForm)
	a := newFakePlayer(7, insideAt)

	Revalidate(z1, a)
	Revalidate(z2, a)
	Remove(z1, a)
	if !a.flags.Has(FlagPeace) {
		t.Fatal("leaving one of two overlapping peace zones cleared the flag")
	}
	Remove(z2, a)
	if a.flags.Has(FlagPeace) {
		t.Fatal("leaving both peace zones left the flag raised")
	}
}

func TestDerbyTrackOnlyFlagsPlayables(t *testing.T) {
	z := NewDerbyTrack(1, testForm)
	npc := &fakeActor{id: 8, pos: insideAt, class: ClassNPC}
	Revalidate(z, npc)
	if !z.Inside(npc) {
		t.Fatal("NPC not tracked by the race track zone")
	}
	for _, f := range []Flag{FlagMonsterTrack, FlagPeace, FlagNoSummonFriend} {
		if npc.flags.Has(f) {
			t.Errorf("NPC got playable-only flag %v", f)
		}
	}
}

func TestMotherTreeRaceGate(t *testing.T) {
	set := statSet(map[string]string{"affectedRace": "1", "enterMsgId": "77"})
	z, err := NewMotherTree(1, testForm, set)
	if err != nil {
		t.Fatal(err)
	}
	var told []int
	z.Notify = func(_ Actor, id int) { told = append(told, id) }

	wrongRace := newFakePlayer(7, insideAt)
	wrongRace.race = player.RaceHuman
	Revalidate(z, wrongRace)
	if z.Inside(wrongRace) {
		t.Fatal("player of another race was affected by the race-gated zone")
	}

	elf := newFakePlayer(8, insideAt)
	elf.race = player.RaceElf
	Revalidate(z, elf)
	if !z.Inside(elf) || !elf.flags.Has(FlagMotherTree) {
		t.Fatal("player of the configured race was not affected")
	}
	if len(told) != 1 || told[0] != 77 {
		t.Fatalf("entry announcement = %v, want [77]", told)
	}

	// Non-players bypass the race gate entirely.
	npc := &fakeActor{id: 9, pos: insideAt, class: ClassNPC}
	Revalidate(z, npc)
	if !z.Inside(npc) {
		t.Fatal("non-player was blocked by the race gate")
	}
}

func TestTownCombatRules(t *testing.T) {
	newTown := func(rule int) *Town {
		z, err := NewTown(1, testForm, statSet(map[string]string{"townId": "9", "castleId": "3"}))
		if err != nil {
			t.Fatal(err)
		}
		z.CombatRule = rule
		return z
	}

	// Default rule: peace and town flags both raise.
	a := newFakePlayer(7, insideAt)
	Revalidate(newTown(0), a)
	if !a.flags.Has(FlagPeace) || !a.flags.Has(FlagTown) {
		t.Error("default town did not raise peace and town flags")
	}

	// Rule 2: town flag only.
	b := newFakePlayer(8, insideAt)
	Revalidate(newTown(2), b)
	if b.flags.Has(FlagPeace) {
		t.Error("rule-2 town raised the peace flag")
	}
	if !b.flags.Has(FlagTown) {
		t.Error("rule-2 town did not raise the town flag")
	}

	// Rule 1 with a siege participant: neither flag.
	c := newFakePlayer(9, insideAt)
	z := newTown(1)
	z.InSiege = func(Actor) bool { return true }
	Revalidate(z, c)
	if c.flags.Has(FlagPeace) || c.flags.Has(FlagTown) {
		t.Error("rule-1 town flagged a siege participant")
	}
}

func TestConcurrentRevalidateIsSafe(t *testing.T) {
	z := NewPeace(1, testForm)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			a := newFakePlayer(id, insideAt)
			for j := 0; j < 100; j++ {
				Revalidate(z, a)
				Remove(z, a)
			}
		}(int32(i))
	}
	wg.Wait()
	if got := len(z.Occupants()); got != 0 {
		t.Fatalf("%d occupants left after everyone was removed", got)
	}
}
