package duel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The zone test duellists teleport into a cuboid 1000 to 1900 east of the
// class template's spawn point, inside the 2000 a duel allows between its
// players. The fixture teleports in because a teleport's arrival revalidates
// zones at once.
const zoneStepX, zoneStepY = 1500, 20

// arenaWithZone boots an arena whose world holds one zone over the step.
func arenaWithZone(t *testing.T, add func(*zone.Index, zone.Form), names ...string) *arena {
	t.Helper()
	return arenaWithZoneAt(t, 1000, 1900, add, names...)
}

// arenaWithZoneAt is arenaWithZone with the cuboid spanning minX to maxX
// east to west and every y.
func arenaWithZoneAt(t *testing.T, minX, maxX int, add func(*zone.Index, zone.Form), names ...string) *arena {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -1000000, 1000000, -10000, 10000)
	if err != nil {
		t.Fatal(err)
	}
	index := zone.NewIndex()
	add(index, form)
	return bootArenaWith(t, []gameservertest.Option{gameservertest.WithZones(index)}, nil, names...)
}

// stepIntoZone puts player i on the step.
func (a *arena) stepIntoZone(t *testing.T, i int) {
	t.Helper()
	a.teleportTo(t, i, zoneStepX, zoneStepY)
}

// teleportTo puts player i at x, y and has its client appear there.
func (a *arena) teleportTo(t *testing.T, i, x, y int) {
	t.Helper()
	_, _, z := a.srv.PlayerPosition(t, a.players[i].id)
	obj, ok := a.srv.State.Player(a.players[i].id)
	if !ok {
		t.Fatalf("player %d not in world", a.players[i].id)
	}
	char, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	a.onQueue(t, i, func() { char.TeleportTo(x, y, z, 0) })
	a.players[i].c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
}

// TestDuelCancelledInABlockedZone: a duellist that steps into a peace
// zone, a running siege's zone or a PvP zone ends its duel as a tie for
// both, the check running on the duel's own tick.
func TestDuelCancelledInABlockedZone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		add  func(*zone.Index, zone.Form)
	}{
		{"peace", func(i *zone.Index, f zone.Form) { i.Add(zone.NewPeace(1, f)) }},
		{"siege", func(i *zone.Index, f zone.Form) {
			s := zone.NewSiege(1, f)
			s.SetActive(true)
			i.Add(s)
		}},
		{"pvp", func(i *zone.Index, f zone.Form) { i.Add(zone.NewArena(1, f)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := arenaWithZone(t, tc.add, "Challenger", "Rival")
			a.challenge(t, 0, 1)
			a.stepIntoZone(t, 1)
			a.duelEnded(t, "duel cancelled")
			for _, p := range a.players {
				wantTie(t, p.name, drainFrames(t, p.c))
			}
		})
	}
}

// TestDuelKeepsGoingOutsideTheZone is the control: the same step, with no
// zone there, leaves the duel running.
func TestDuelKeepsGoingOutsideTheZone(t *testing.T) {
	t.Parallel()
	a := arenaWithZone(t, func(*zone.Index, zone.Form) {}, "Challenger", "Rival")
	a.challenge(t, 0, 1)
	a.stepIntoZone(t, 1)
	a.srv.Advance(t, 5*time.Second)
	if s := a.standing(t, 0); s.DuelState() != duel.Duelling {
		t.Fatalf("duel state after the step = %d, want Duelling", s.DuelState())
	}
}

// TestPartyDuelCancelledWhenAMemberEntersAPeaceZone: one member of a
// duelling party stepping into a peace zone ends the duel as a tie for
// both parties, not only for the member's own.
func TestPartyDuelCancelledWhenAMemberEntersAPeaceZone(t *testing.T) {
	t.Parallel()
	// The zone lies 400 east of the arena the party duel moves everyone to.
	a := arenaWithZoneAt(t, -83600, -83000, func(i *zone.Index, f zone.Form) { i.Add(zone.NewPeace(1, f)) },
		"Lead", "Mate", "Foe", "Ally")
	a.party(t, 0, 1)
	a.party(t, 2, 3)
	a.startPartyDuel(t)
	a.srv.Advance(t, 30*time.Second)
	a.srv.AdvanceUntil(t, "duel begins", func() bool { return a.allIn(t, duel.Duelling) })
	a.quiet(t)

	a.teleportTo(t, 3, -83300, -238675)
	a.srv.AdvanceUntil(t, "duel cancelled", func() bool {
		for i := range a.players {
			if a.standing(t, i).InDuel() {
				return false
			}
		}
		return true
	})
	for _, p := range a.players {
		wantTie(t, p.name, drainFrames(t, p.c))
	}
}
