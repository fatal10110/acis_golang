package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestWalkBetweenAdjoiningWaterZonesKeepsSwimming pins a player walking
// across two touching water zones (#4954): in either direction, the old
// zone's exit leaves the player swimming because the other zone still holds
// it, so it keeps the swim move type and its breath countdown runs on
// without a SetupGauge reset. Walking out of the last water zone stops the
// swim, sends UserInfo and SetupGauge(0), and no drowning follows.
func TestWalkBetweenAdjoiningWaterZonesKeepsSwimming(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	for _, z := range []struct{ id, minX, maxX int }{{1, -1_000, 100}, {2, 101, 600}} {
		form, err := zone.NewCuboid(z.minX, z.maxX, -1_000, 1_000, -1_000, 150)
		if err != nil {
			t.Fatal(err)
		}
		zones.Add(zone.NewWater(z.id, form))
	}
	clock := &waterClock{}
	clock.nanos.Store(time.Now().UnixNano())
	srv, character, objID := bootInZones(t, zones, gameservertest.WithWater(clock.now))
	if !srv.DrivesClock() {
		t.Skip("stepping a walk needs the driven clock")
	}
	mover := srv.PlayerMove(t, objID)
	if at := mover.Position(); at.X >= 100 || !character.InWater() || mover.MoveType() != move.MoveSwim {
		t.Fatalf("spawn at %+v in water %v move type %d, want swimming in the west zone", at, character.InWater(), mover.MoveType())
	}

	walk := func(x int) [][]byte {
		t.Helper()
		from := mover.Position()
		dest := location.Location{X: x, Y: from.Y, Z: from.Z}
		c := srv.Client
		c.Send(encodeMoveBackwardToLocation(dest, from, 1))
		frames := testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList)) }, serverpackets.OpcodeItemList)
		for update := 0; mover.Moving(); update++ {
			if update == 1_000 {
				t.Fatalf("walk to %+v still under way after 1000 updates", dest)
			}
			srv.TickPositions()
		}
		frames = append(frames, testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList)) }, serverpackets.OpcodeItemList)...)
		if firstOpcode(frames, serverpackets.OpcodeMoveToLocation) < 0 {
			t.Fatalf("walk to %+v sent no MoveToLocation", dest)
		}
		if at := mover.Position(); at.X != x {
			t.Fatalf("walk ended at %+v, want X=%d", at, x)
		}
		return frames
	}

	for _, crossing := range []struct {
		name string
		x    int
	}{{"west to east", 300}, {"east to west", 10}} {
		frames := walk(crossing.x)
		if !character.InWater() || mover.MoveType() != move.MoveSwim {
			t.Fatalf("%s: in water %v, move type %d; want still swimming", crossing.name, character.InWater(), mover.MoveType())
		}
		if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i >= 0 {
			t.Fatalf("%s: breath gauge reset to %d, want the countdown to run on", crossing.name, gaugeTime(t, frames[i]))
		}
	}

	frames := walk(800)
	if character.InWater() || mover.MoveType() != move.MoveGround {
		t.Fatalf("on dry land: in water %v, move type %d; want walking on the ground", character.InWater(), mover.MoveType())
	}
	if firstOpcode(frames, serverpackets.OpcodeUserInfo) < 0 {
		t.Fatal("leaving the last water zone sent no UserInfo")
	}
	if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i < 0 || gaugeTime(t, frames[i]) != 0 {
		t.Fatal("leaving the last water zone did not stop the breath gauge")
	}
	clock.advance(time.Hour)
	srv.Water.Tick()
	if i := drownIndex(srv.ReadQueued(t, srv.Client)); i >= 0 {
		t.Fatal("drowning damage after leaving the last water zone")
	}
}
