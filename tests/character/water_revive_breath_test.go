package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestReviveInWaterRestartsBreathOnNextMove pins Player.revalidateZone
// (Player.java:843-853): every revalidation, each position update of a
// move included (PlayerMove.java:321), adds a living swimmer to
// WaterTaskManager, which starts a countdown for an untracked one
// (WaterTaskManager.java:62-76). Death stops the countdown; a player revived
// where it died, still swimming, gets a fresh breath gauge on its first
// position update and drowns again once that breath runs out.
func TestReviveInWaterRestartsBreathOnNextMove(t *testing.T) {
	t.Parallel()
	srv, character, objID, clock := bootInWater(t)
	if !srv.DrivesClock() {
		t.Skip("stepping position updates needs the driven clock")
	}
	if !character.InWater() {
		t.Fatal("player not in water at spawn")
	}

	if !character.Die(nil) {
		t.Fatal("Die() = false for a living player")
	}
	frames := srv.ReadQueued(t, srv.Client)
	if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i < 0 || gaugeTime(t, frames[i]) != 0 {
		t.Fatalf("death sent no SetupGauge(0) among %d frames", len(frames))
	}
	clock.advance(time.Hour)
	srv.Water.Tick()
	if i := drownIndex(srv.ReadQueued(t, srv.Client)); i >= 0 {
		t.Fatal("a dead player took drowning damage")
	}

	if !character.Revive() {
		t.Fatal("Revive() = false for a dead player")
	}
	frames = srv.ReadQueued(t, srv.Client)
	if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i >= 0 {
		t.Fatalf("revive sent a breath gauge (frame %d), want it at the next revalidation", i)
	}
	srv.Water.Tick()
	if i := drownIndex(srv.ReadQueued(t, srv.Client)); i >= 0 {
		t.Fatal("drowning damage before any revalidation after the revive")
	}

	x, y, z := srv.PlayerPosition(t, objID)
	from := location.Location{X: x, Y: y, Z: z}
	srv.Client.Send(encodeMoveBackwardToLocation(location.Location{X: x + 500, Y: y, Z: z}, from, 1))
	expectGroundClickAck(t, srv.Client)
	if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("move opcode = %#x, want MoveToLocation", reply[0])
	}
	frames = tickFrames(t, srv)
	gauge := firstOpcode(frames, serverpackets.OpcodeSetupGauge)
	if gauge < 0 {
		t.Fatal("first position update after the revive sent no breath gauge")
	}
	if got := gaugeTime(t, frames[gauge]); got <= 0 {
		t.Fatalf("SetupGauge time after the revive = %d, want a fresh full breath", got)
	}
	if !character.InWater() {
		t.Fatal("player left the water")
	}

	clock.advance(time.Hour)
	srv.Water.Tick()
	if drownIndex(srv.ReadQueued(t, srv.Client)) < 0 {
		t.Fatal("no drowning damage once the restarted breath ran out")
	}
}
