package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Rune's dock, where runeItinerary's boat ties up, and a point on its shore
// whose walk to the dock crosses the dock's entrance.
var (
	runeDock  = location.Location{X: 34381, Y: -37680, Z: -3610}
	runeShore = location.Location{X: 34600, Y: -38100, Z: -3610}
)

// moveToTargetAction is the pet command to approach the owner's target.
const moveToTargetAction = int32(53)

// runeItinerary is a boat sailing from Rune and back, tied up at Rune when
// the server boots.
func runeItinerary() route.BoatItinerary {
	at := func(dx, dy int) route.BoatLocation {
		return route.BoatLocation{Location: location.Location{X: runeDock.X + dx, Y: runeDock.Y + dy, Z: runeDock.Z}, Speed: 200, Rotation: 800}
	}
	return route.BoatItinerary{Heading: 40785, Routes: []route.BoatRoute{
		{Dock: route.DockRune, Nodes: []route.BoatLocation{at(-1000, 0), at(-2000, 0)}},
		{Dock: route.DockPrimeval, Nodes: []route.BoatLocation{at(-1000, 0), at(0, 0)}},
	}}
}

// bootPetOwnerAt brings the owner into the world standing at at, with its
// wolf out, and a folk NPC standing at folkAt selected as its target. The
// owner's client is left quiet.
func bootPetOwnerAt(t *testing.T, at, folkAt location.Location, opts ...gameservertest.Option) (*petWorld, *summon.Actor) {
	t.Helper()
	srv := bootPets(t, opts...)
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, ownerID); err != nil {
		t.Fatalf("place owner: %v", err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: srv.GiveItem(t, ownerID, wolfCollarID, 1), seeded: map[int32][]int32{}}
	// The entry burst near a dock carries the boat too; it is not pinned
	// here.
	h.client.Send(encodeRequestGameStart(0))
	mustRead(t, h.client, "SSQInfo")
	mustRead(t, h.client, "CharSelected")
	h.client.Send(encodeEnterWorld())
	drainUntilQuiet(t, h.client)
	pet, _ := h.spawnWolf(t)
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31031), folkAt)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(folk.ObjectID(), int32(folkAt.X), int32(folkAt.Y), int32(folkAt.Z), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMyTargetSelected, "folk selected")
	drainUntilQuiet(t, h.client)
	return h, pet
}

// petMoves returns the destinations of the MoveToLocation frames of pet
// among frames, and how many ActionFailed frames there are.
func petMoves(frames [][]byte, pet int32) (dests []location.Location, actionFailed int) {
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed:
			actionFailed++
		case serverpackets.OpcodeMoveToLocation:
			r := wire.NewReader(f[1:])
			if r.ReadInt32() != pet {
				continue
			}
			dests = append(dests, location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())})
		}
	}
	return dests, actionFailed
}

// TestPetFollowOfOtherStaysWithoutBoatEntrance pins SummonMove's friendly
// follow of anyone but the owner (SummonMove.java:94-99): it never walks
// after the target; with no boat in sight Playable.tryToPassBoatEntrance
// answers the owner ActionFailed and the pet stays where it is.
func TestPetFollowOfOtherStaysWithoutBoatEntrance(t *testing.T) {
	t.Parallel()
	h, pet := bootPetOwnerAt(t, runeShore, offset(runeShore, 400, 0))
	x, y, z := pet.Position()

	h.client.Send(encodeRequestActionUse(moveToTargetAction, false))
	dests, failed := petMoves(drainFrames(t, h.client), pet.ObjectID())
	if len(dests) != 0 || failed != 1 {
		t.Fatalf("pet moves %v, ActionFailed %d; want no move, one ActionFailed", dests, failed)
	}
	if gx, gy, gz := pet.Position(); gx != x || gy != y || gz != z {
		t.Fatalf("pet at %d,%d,%d, want still at %d,%d,%d", gx, gy, gz, x, y, z)
	}
}

// TestPetFollowOfOtherWalksToBoatEntrance pins the other half: when the
// pet's walk toward the target crosses the entrance of the dock a boat it
// knows is tied up at, it walks to that entrance on the shore line
// (Playable.moveToBoatEntrance), and the owner gets no ActionFailed.
func TestPetFollowOfOtherWalksToBoatEntrance(t *testing.T) {
	t.Parallel()
	h, pet := bootPetOwnerAt(t, runeShore, runeDock, gameservertest.WithBoats(runeItinerary()))
	x, y, _ := pet.Position()
	point, ok := h.srv.Boats.Boats()[0].Dock().BoardingPoint(boat.Point{X: x, Y: y}, boat.Point{X: runeDock.X, Y: runeDock.Y}, false)
	if !ok {
		t.Fatalf("the pet's walk from %d,%d to the dock crosses no entrance", x, y)
	}

	h.client.Send(encodeRequestActionUse(moveToTargetAction, false))
	dests, failed := petMoves(drainFrames(t, h.client), pet.ObjectID())
	if len(dests) != 1 || dests[0].X != point.X || dests[0].Y != point.Y || failed != 0 {
		t.Fatalf("pet moves %v, ActionFailed %d; want one walk to the entrance %+v, no ActionFailed", dests, failed, point)
	}
}

// TestPetFollowOfOtherStaysWhenNoEntranceCrossed pins the refusal of
// Playable.tryToPassBoatEntrance with a boat in sight: the pet's walk toward
// the target crosses no entrance of the boat's dock, so the owner is
// answered ActionFailed and the pet stays where it is. The pet stands where
// TestPetFollowOfOtherWalksToBoatEntrance's does, in sight of the boat.
func TestPetFollowOfOtherStaysWhenNoEntranceCrossed(t *testing.T) {
	t.Parallel()
	folkAt := offset(runeShore, 400, 0)
	h, pet := bootPetOwnerAt(t, runeShore, folkAt, gameservertest.WithBoats(runeItinerary()))
	x, y, z := pet.Position()
	if _, ok := h.srv.Boats.Boats()[0].Dock().BoardingPoint(boat.Point{X: x, Y: y}, boat.Point{X: folkAt.X, Y: folkAt.Y}, false); ok {
		t.Fatalf("the pet's walk from %d,%d to the folk crosses the dock's entrance", x, y)
	}

	h.client.Send(encodeRequestActionUse(moveToTargetAction, false))
	dests, failed := petMoves(drainFrames(t, h.client), pet.ObjectID())
	if len(dests) != 0 || failed != 1 {
		t.Fatalf("pet moves %v, ActionFailed %d; want no move, one ActionFailed", dests, failed)
	}
	if gx, gy, gz := pet.Position(); gx != x || gy != y || gz != z {
		t.Fatalf("pet at %d,%d,%d, want still at %d,%d,%d", gx, gy, gz, x, y, z)
	}
}

// TestPetFollowOfOtherStaysAtBoatEntrance pins Playable.moveToBoatEntrance's
// other branch: the pet's walk toward the target crosses the entrance, but
// the shore point lies within 50 of the pet, so the owner is answered
// ActionFailed and the pet is not walked to where it already stands.
func TestPetFollowOfOtherStaysAtBoatEntrance(t *testing.T) {
	t.Parallel()
	h, pet := bootPetOwnerAt(t, runeShore, runeDock, gameservertest.WithBoats(runeItinerary()))
	// Just shore-side of the middle of Rune's entrance line.
	at := location.Location{X: 34475, Y: -37950, Z: runeDock.Z}
	runOn(t, pet.Queue(), func() { pet.SetXYZ(at.X, at.Y, at.Z) })
	drainUntilQuiet(t, h.client)
	x, y, z := pet.Position()
	point, ok := h.srv.Boats.Boats()[0].Dock().BoardingPoint(boat.Point{X: x, Y: y}, boat.Point{X: runeDock.X, Y: runeDock.Y}, false)
	if !ok {
		t.Fatalf("the pet's walk from %d,%d to the dock crosses no entrance", x, y)
	}
	if d := (location.Location{X: x, Y: y}).Distance2D(location.Location{X: point.X, Y: point.Y}); d > 50 {
		t.Fatalf("the entrance %+v lies %.0f from the pet, want within 50", point, d)
	}

	h.client.Send(encodeRequestActionUse(moveToTargetAction, false))
	dests, failed := petMoves(drainFrames(t, h.client), pet.ObjectID())
	if len(dests) != 0 || failed != 1 {
		t.Fatalf("pet moves %v, ActionFailed %d; want no move, one ActionFailed", dests, failed)
	}
	if gx, gy, gz := pet.Position(); gx != x || gy != y || gz != z {
		t.Fatalf("pet at %d,%d,%d, want still at %d,%d,%d", gx, gy, gz, x, y, z)
	}
}

func offset(l location.Location, dx, dy int) location.Location {
	return location.Location{X: l.X + dx, Y: l.Y + dy, Z: l.Z}
}
