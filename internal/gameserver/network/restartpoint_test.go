package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// townRestartTable returns a restart table whose only point covers the map
// region a player spawned near the world origin falls into.
func townRestartTable() *restart.Table {
	return &restart.Table{
		Points: []restart.Point{
			{
				Name:       "town",
				Points:     []location.Location{{X: 5000, Y: 5000, Z: 100}},
				MapRegions: []location.Point{{X: 20, Y: 18}},
			},
		},
	}
}

func TestRestartLivePlayerIgnoresLivingPlayer(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	frames.frames = nil

	gcl := &GameClientLink{world: state, geo: testGeo{}, restarts: townRestartTable(), respawnRestoreHP: 0.7, log: zerolog.Nop()}
	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{})

	if len(frames.frames) != 0 {
		t.Fatalf("frames sent for a living player = %d, want 0", len(frames.frames))
	}
}

func TestRestartLivePlayerRevivesAndTeleportsDeadPlayer(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.SetHP(1)
	if !live.Die(nil) {
		t.Fatal("precondition: Die() = false, want true")
	}
	frames.frames = nil

	restarts := townRestartTable()
	gcl := &GameClientLink{world: state, geo: testGeo{}, restarts: restarts, respawnRestoreHP: 0.7, log: zerolog.Nop()}
	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{RequestType: 0})

	if live.Dead() {
		t.Fatal("Dead() = true after restart, want false (revived)")
	}
	if !live.Teleporting() {
		t.Fatal("Teleporting() = false after restart teleport started")
	}

	wantOpcodes := []byte{serverpackets.OpcodeRevive, serverpackets.OpcodeTeleportToLocation}
	if got := frameOpcodes(frames.frames); string(got) != string(wantOpcodes) {
		t.Fatalf("opcodes = %x, want Revive then TeleportToLocation (%x)", got, wantOpcodes)
	}

	dest, _ := restarts.NearestLocation(location.Location{}, live.Race, live.Karma())
	got := live.CurrentLocation()
	if dx := got.X - dest.X; dx < -restartTeleportOffset || dx > restartTeleportOffset {
		t.Fatalf("teleported X = %d, want within %d of %d", got.X, restartTeleportOffset, dest.X)
	}
	if dy := got.Y - dest.Y; dy < -restartTeleportOffset || dy > restartTeleportOffset {
		t.Fatalf("teleported Y = %d, want within %d of %d", got.Y, restartTeleportOffset, dest.Y)
	}
}

// TestRestartLivePlayerWithNoRestartTableSendsActionFailed pins the
// data-missing fallback: when the restart-point table didn't load at all, the
// dead player can't be revived or teleported, and silently answering nothing
// would strand them on the death screen. The reference path always resolves
// at least the nearest town, so this case is a Go-side data-loading gap, not a
// rejection the reference makes — it falls back to ActionFailed so the client
// can dismiss the pending death action and stays dead past the warn in the log.
func TestRestartLivePlayerWithNoRestartTableSendsActionFailed(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.SetHP(1)
	live.Die(nil)
	frames.frames = nil

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{})

	if got := frameOpcodes(frames.frames); len(got) != 1 || got[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [ActionFailed]", got)
	}
	if !live.Dead() {
		t.Fatal("Dead() = false with no restart destination resolved, want still dead")
	}
}
