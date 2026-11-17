package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Residence.openDoor and closeDoor act on the residence's own
// doors only (getDoor looks the id up among them); the flag war's
// getKnownTypeInside(ClanHallZone) lists the players in the zones of the
// hall's residence.

const (
	// The doors beside the spawn point: an outter gate of Devastated Castle
	// (its name in capitals: gate names are matched without case), and two
	// doors of no hall.
	outterGateID       = 25_170_101
	strangerOpenID     = 25_170_201
	strangerClosedID   = 25_170_202
	otherHallResidence = 35
)

// doorAt is a door of the given name standing at x, y beside the spawn.
func doorAt(id int, name string, x int, opened bool) *door.Template {
	return &door.Template{
		ID: id, Name: name, Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
		Coordinates: []location.Point{
			{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened:   opened,
		OpenKind: door.OpenNPC,
	}
}

// TestHallScriptOpensAndClosesOnlyTheHallsGates: a script opens and closes
// the doors named among the hall's gates, whatever the case of the door's
// name, and leaves a door that is none of them as it stands, closed or
// open; an id of no door does nothing.
func TestHallScriptOpensAndClosesOnlyTheHallsGates(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{owned: true, extra: []gameservertest.Option{
		seamScripts(),
		gameservertest.WithDoors(
			doorAt(outterGateID, "DEVASTATED_CASTLE_OUTTER_001", 200, false),
			doorAt(strangerOpenID, "Bandit_stronghold_001", 300, true),
			doorAt(strangerClosedID, "Devastated_castle_005", 400, false),
		),
	}})
	open := func(id int) bool {
		t.Helper()
		_, o := w.srv.DoorState(t, id)
		return o
	}
	if !open(devastatedGateID) || open(outterGateID) || !open(strangerOpenID) || open(strangerClosedID) {
		t.Fatalf("doors before the script: gate %v, outter gate %v, stranger %v, closed stranger %v; want open, closed, open, closed",
			open(devastatedGateID), open(outterGateID), open(strangerOpenID), open(strangerClosedID))
	}

	hall := func(fn func(h *script.Hall)) {
		t.Helper()
		if !w.srv.RunScript(t, w.objID, seamQuest, func(s *script.Script, _ *script.Player) {
			h := s.SiegableHall(devastatedCastle)
			if h == nil {
				t.Error("Devastated Castle reads as no siegable hall")
				return
			}
			fn(h)
		}) {
			t.Fatal("the script's invocation panicked")
		}
	}
	hall(func(h *script.Hall) {
		h.CloseDoor(devastatedGateID)
		h.OpenDoor(outterGateID)
		h.CloseDoor(strangerOpenID)
		h.OpenDoor(strangerClosedID)
		h.OpenDoor(1)
		h.CloseDoor(1)
	})
	if open(devastatedGateID) || !open(outterGateID) {
		t.Errorf("after the close and the open: gate open %v, outter gate open %v; want closed, open", open(devastatedGateID), open(outterGateID))
	}
	if !open(strangerOpenID) || open(strangerClosedID) {
		t.Errorf("after the hall's calls on other doors: open stranger %v, closed stranger %v; want them as they were, open and closed",
			open(strangerOpenID), open(strangerClosedID))
	}

	hall(func(h *script.Hall) {
		h.OpenDoor(devastatedGateID)
		h.CloseDoor(outterGateID)
	})
	if !open(devastatedGateID) || open(outterGateID) {
		t.Errorf("after the reverse calls: gate open %v, outter gate open %v; want open, closed", open(devastatedGateID), open(outterGateID))
	}
}

// groundsAndRivals is the zones of a hall world: the hall's battlefield
// and its grounds over the spawn point, and the grounds of another hall
// over it too.
func groundsAndRivals(t *testing.T, groundsOverSpawn bool) *zone.Index {
	t.Helper()
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = int(devastatedCastle)
	far := cuboid(t, 5000, 6000, 5000, 6000)
	over := cuboid(t, -200, 200, -200, 200)
	grounds := zone.NewClanHall(2, far)
	if groundsOverSpawn {
		grounds = zone.NewClanHall(2, over)
	}
	grounds.ResidenceID = int(devastatedCastle)
	rival := zone.NewClanHall(3, over)
	rival.ResidenceID = otherHallResidence
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	zones.Add(rival)
	return zones
}

// TestHallPlayersInsideAreThoseInTheHallsGrounds: a script reads the
// players standing in the clan hall zones of its own hall, each once; a
// player in the grounds of another hall, or in none, is not among them.
func TestHallPlayersInsideAreThoseInTheHallsGrounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		inside bool
	}{
		{"in the hall's grounds", true},
		{"in another hall's grounds only", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootHallSiege(t, hallSiegeSetup{owned: true, extra: []gameservertest.Option{
				seamScripts(), gameservertest.WithZones(groundsAndRivals(t, tc.inside)),
			}})
			var inside []int32
			if !w.srv.RunScript(t, w.objID, seamQuest, func(s *script.Script, _ *script.Player) {
				h := s.SiegableHall(devastatedCastle)
				if h == nil {
					t.Error("Devastated Castle reads as no siegable hall")
					return
				}
				for _, p := range h.PlayersInside() {
					inside = append(inside, p.ObjectID())
				}
			}) {
				t.Fatal("the script's invocation panicked")
			}
			want := []int32{}
			if tc.inside {
				want = []int32{w.objID}
			}
			if len(inside) != len(want) || (len(want) == 1 && inside[0] != want[0]) {
				t.Fatalf("players inside Devastated Castle = %v, want %v", inside, want)
			}
		})
	}
}
