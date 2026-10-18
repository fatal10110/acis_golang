package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Castle trap zones show a client map effect (CastleZoneType.java:38-80,
// WorldObject.setRegion, WorldObject.java:377-411): when a region leaves a
// player's surroundings, each castle trap zone touching that region with an
// eventId sends that player EventTrigger(eventId, false), once per region;
// when a region enters them, only an enabled trap sends
// EventTrigger(eventId, true). Nothing enables a castle trap, so in play
// only the remove half is ever seen.

// eventTrigger is one parsed EventTrigger frame.
type eventTrigger struct {
	id     int32
	active bool
}

// eventTriggers returns the EventTrigger frames among frames, in order.
func eventTriggers(frames [][]byte) []eventTrigger {
	var out []eventTrigger
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeEventTrigger {
			continue
		}
		r := wire.NewReader(f[1:])
		out = append(out, eventTrigger{id: r.ReadInt32(), active: r.ReadUint8() == 1})
	}
	return out
}

func assertEventTriggers(t *testing.T, when string, frames [][]byte, want ...eventTrigger) {
	t.Helper()
	got := eventTriggers(frames)
	if len(got) != len(want) {
		t.Fatalf("%s: EventTrigger frames = %+v, want %+v", when, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: EventTrigger frames = %+v, want %+v", when, got, want)
		}
	}
}

// eventSwamp builds a swamp over the given box showing map effect eventID.
func eventSwamp(t *testing.T, id, eventID, minX, maxX, minY, maxY int) *zone.Swamp {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, minY, maxY, -10_000, 10_000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	s := zone.NewSwamp(id, form)
	s.CastleID = 5
	s.EventID = eventID
	return s
}

// TestWalkOutOfCastleTrapRegionTurnsItsEffectOff: the walker spawns at
// X=10, so its surroundings span X -2048..4095 and include the region
// X -2048..-1 the trap lies in. Entering the world shows nothing, since the
// trap is disabled. Walking east, the walker enters the region at X=2048,
// whose surroundings no longer include the trap's region: that step alone
// sends the trap's EventTrigger off, once.
func TestWalkOutOfCastleTrapRegionTurnsItsEffectOff(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(eventSwamp(t, 70001, 2418001, -1_900, -100, 100, 1_900))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Walker", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	if !srv.DrivesClock() {
		t.Skip("stepping a walk tick by tick needs the driven clock")
	}
	c := srv.Client
	objID := srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	assertEventTriggers(t, "entering the world", c.ReadQueued())

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(location.Location{X: 9_000, Y: 20, Z: 30}, spawn, 1))
	frames := c.ReadQueued()
	mover := srv.PlayerMove(t, objID)
	for mover.Position().X < 2_048 {
		if !mover.Moving() {
			t.Fatalf("walk stopped at %v before crossing X=2048", mover.Position())
		}
		frames = append(frames, c.ReadQueued()...)
		srv.TickPositions()
	}
	assertEventTriggers(t, "walking inside the trap's surroundings", frames)
	assertEventTriggers(t, "crossing X=2048", c.ReadQueued(), eventTrigger{2418001, false})
}

// TestTeleportAwayFromCastleTrapsTurnsTheirEffectsOff: at X=54272 the
// player's surroundings are the columns X 51200..57343. Trap A lies in the
// first column, trap B spans the second and third, trap C, armed, lies in
// the first after A, and a swamp without an eventId lies there too. The
// Appearing rejoin shows only the armed trap; the next teleport's grid leave
// turns every trap's effect off, region by region: A and C for the first
// column, then B once for each of its two columns.
func TestTeleportAwayFromCastleTrapsTurnsTheirEffectsOff(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(eventSwamp(t, 70001, 2418001, 51_300, 53_000, 100, 1_900))
	zones.Add(eventSwamp(t, 70002, 2418002, 54_000, 56_000, 100, 1_900))
	armed := eventSwamp(t, 70003, 2418003, 51_300, 53_000, 100, 1_900)
	armed.Armed = true
	zones.Add(armed)
	zones.Add(eventSwamp(t, 70004, 0, 51_300, 53_000, 100, 1_900))
	srv, character, _ := bootInZones(t, zones)

	character.TeleportTo(54_272, 1_000, 30, 0)
	assertEventTriggers(t, "teleporting next to the traps", srv.Client.ReadQueued())
	assertEventTriggers(t, "appearing next to the traps", appear(t, srv.Client), eventTrigger{2418003, true})

	character.TeleportTo(10, 20, 30, 0)
	assertEventTriggers(t, "teleporting away", srv.Client.ReadQueued(),
		eventTrigger{2418001, false}, eventTrigger{2418003, false},
		eventTrigger{2418002, false}, eventTrigger{2418002, false})
	assertEventTriggers(t, "appearing away", appear(t, srv.Client))
}
