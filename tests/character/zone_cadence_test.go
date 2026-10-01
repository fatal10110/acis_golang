package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// besideWaterSpawn is where the seeded character spawns.
var besideWaterSpawn = location.Location{X: 10, Y: 20, Z: 30}

// bootBesideWater boots a player standing just west of a water zone that
// starts 5 units east of it, with the movement clock driven by hand.
func bootBesideWater(t *testing.T) (*gameservertest.Server, *player.Character, int32) {
	t.Helper()
	spawn := besideWaterSpawn
	srv, character, objID := bootInZones(t, waterZones(t, spawn.X+5, spawn.X+5_000))
	if !srv.DrivesClock() {
		t.Skip("counting position updates needs the driven clock")
	}
	if x, y, z := srv.PlayerPosition(t, objID); (location.Location{X: x, Y: y, Z: z}) != spawn {
		t.Fatalf("spawned at (%d, %d, %d), want %+v", x, y, z, spawn)
	}
	if character.InWater() {
		t.Fatal("player in water at spawn")
	}
	return srv, character, objID
}

// tickFrames runs one position update and returns every frame it sent the
// player, fenced by an ItemList barrier.
func tickFrames(t *testing.T, srv *gameservertest.Server) [][]byte {
	t.Helper()
	srv.TickPositions()
	c := srv.Client
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList)) }, serverpackets.OpcodeItemList)
}

// TestZoneEnterWaitsForFifthPositionUpdate pins Creature.revalidateZone
// (Creature.java:1313-1330): each position update of a walk revalidates the
// zones with force false (PlayerMove.java:321), which only runs on every
// fifth call since the last forced one (the EnterWorld revalidation,
// Player.java:5982). A player who walks into a water zone on its first
// update therefore keeps walking on the ground, without the entry UserInfo
// (WaterZone.onEnter), until the fifth; there the zone adds SWIM and the
// player swims.
func TestZoneEnterWaitsForFifthPositionUpdate(t *testing.T) {
	srv, character, objID := bootBesideWater(t)
	spawn := besideWaterSpawn
	mover := srv.PlayerMove(t, objID)
	srv.Client.Send(encodeMoveBackwardToLocation(location.Location{X: spawn.X + 3_000, Y: spawn.Y, Z: spawn.Z}, spawn, 1))
	if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", reply[0])
	}

	for update := 1; update <= 4; update++ {
		frames := tickFrames(t, srv)
		if at := mover.Position(); at.X < spawn.X+5 {
			t.Fatalf("update %d: player at %+v, short of the water zone", update, at)
		}
		if character.InWater() || mover.MoveType() != move.MoveGround {
			t.Fatalf("update %d: in water %v, move type %d; want the zone entered on the fifth update only", update, character.InWater(), mover.MoveType())
		}
		if i := firstOpcode(frames, serverpackets.OpcodeUserInfo); i >= 0 {
			t.Fatalf("update %d sent UserInfo before the zone revalidated: %x", update, frames)
		}
	}
	frames := tickFrames(t, srv)
	if !character.InWater() || mover.MoveType() != move.MoveSwim {
		t.Fatalf("fifth update: in water %v, move type %d; want swimming", character.InWater(), mover.MoveType())
	}
	if firstOpcode(frames, serverpackets.OpcodeUserInfo) < 0 {
		t.Fatalf("fifth update frames = %x, want the water entry UserInfo", frames)
	}
}

// TestZoneEnterOnArrivalBeforeFifthUpdate pins the forced revalidation at a
// move's end (CreatureMove.java:224-233: revalidateZone(true) before
// ARRIVED): a walk that ends inside the water zone on its second update
// enters it there, without waiting for the fifth.
func TestZoneEnterOnArrivalBeforeFifthUpdate(t *testing.T) {
	srv, character, objID := bootBesideWater(t)
	spawn := besideWaterSpawn
	mover := srv.PlayerMove(t, objID)
	dest := location.Location{X: spawn.X + 12, Y: spawn.Y, Z: spawn.Z}
	srv.Client.Send(encodeMoveBackwardToLocation(dest, spawn, 1))
	if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", reply[0])
	}

	for update := 1; mover.Moving(); update++ {
		if update > 4 {
			t.Fatalf("walk of 12 units still under way after %d updates", update-1)
		}
		tickFrames(t, srv)
	}
	if at := mover.Position(); at.X != dest.X {
		t.Fatalf("walk ended at %+v, want %+v", at, dest)
	}
	if !character.InWater() || mover.MoveType() != move.MoveSwim {
		t.Fatalf("after arrival: in water %v, move type %d; want swimming", character.InWater(), mover.MoveType())
	}
}

// TestZoneEnterOnStopBeforeFifthUpdate pins the forced revalidation of a
// stop (CreatureMove.stop, CreatureMove.java:452-463): CannotMoveAnymore
// stops a walk that entered the water zone on its first update, and the
// zone is entered at once, its UserInfo sent ahead of the StopMove.
func TestZoneEnterOnStopBeforeFifthUpdate(t *testing.T) {
	srv, character, objID := bootBesideWater(t)
	spawn := besideWaterSpawn
	mover := srv.PlayerMove(t, objID)
	c := srv.Client
	c.Send(encodeMoveBackwardToLocation(location.Location{X: spawn.X + 3_000, Y: spawn.Y, Z: spawn.Z}, spawn, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", reply[0])
	}
	tickFrames(t, srv)
	if at := mover.Position(); at.X < spawn.X+5 || character.InWater() {
		t.Fatalf("after one update: at %+v, in water %v; want inside the zone, not entered yet", at, character.InWater())
	}

	frames := testsupport.SyncBarrierFrames(t, c, func() {
		w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveAnymore)
		for range 4 {
			w.WriteInt32(0)
		}
		c.Send(w.Bytes())
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
	if !character.InWater() || mover.MoveType() != move.MoveSwim {
		t.Fatalf("after the stop: in water %v, move type %d; want swimming", character.InWater(), mover.MoveType())
	}
	userInfo, stop := firstOpcode(frames, serverpackets.OpcodeUserInfo), firstOpcode(frames, serverpackets.OpcodeStopMove)
	if userInfo < 0 || stop < 0 || userInfo > stop {
		t.Fatalf("stop frames = %x, want the water entry UserInfo ahead of StopMove", frames)
	}
}

// TestZoneEnterOnRegionCrossingBeforeFifthUpdate pins the forced
// revalidation of a region change (WorldObject.setXYZ → Creature.setRegion,
// Creature.java:1774-1791): a walk inside a water zone it has not entered
// yet enters it on the update that crosses into the next world region, not
// on the fifth.
func TestZoneEnterOnRegionCrossingBeforeFifthUpdate(t *testing.T) {
	// boundary is a world region edge; the player lands 12 short of it,
	// just outside a water zone starting 10 short of it, so its first
	// walk-speed update enters the zone's volume and one of the next three
	// crosses the edge.
	const boundary = 2048
	start := boundary - 12
	srv, character, objID := bootInZones(t, waterZones(t, boundary-10, boundary+5_000))
	if !srv.DrivesClock() {
		t.Skip("counting position updates needs the driven clock")
	}
	mover := srv.PlayerMove(t, objID)
	_, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(start, y, z, 0)
	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	drainQuiet(t, srv.Client)
	x, y, z := srv.PlayerPosition(t, objID)
	from := location.Location{X: x, Y: y, Z: z}
	if character.InWater() || x != start {
		t.Fatalf("after the teleport: at %+v, in water %v; want at X %d, out of the zone", from, character.InWater(), start)
	}

	srv.Client.Send(encodeMoveBackwardToLocation(location.Location{X: start + 3_000, Y: y, Z: z}, from, 1))
	if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", reply[0])
	}
	for update := 1; ; update++ {
		tickFrames(t, srv)
		at := mover.Position()
		if update == 1 && (at.X < boundary-10 || at.X >= boundary) {
			t.Fatalf("first update landed at %+v, want inside the zone short of the region edge %d", at, boundary)
		}
		if at.X < boundary {
			if character.InWater() {
				t.Fatalf("update %d at %+v: zone entered before the region edge", update, at)
			}
			if update >= 4 {
				t.Fatalf("update %d at %+v: walk too slow to cross the region edge before the fifth update", update, at)
			}
			continue
		}
		if !character.InWater() || mover.MoveType() != move.MoveSwim {
			t.Fatalf("update %d crossed the region edge at %+v: in water %v, move type %d; want swimming", update, at, character.InWater(), mover.MoveType())
		}
		return
	}
}
