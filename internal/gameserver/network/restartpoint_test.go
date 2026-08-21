package network

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestTeleportLivePlayerStopsInFlightCast pins the teleport half of the
// actor-state cast-abort surface: a discontinuous relocation stops an
// in-flight cast, the same way it already cancels attack/combat.
func TestTeleportLivePlayerStopsInFlightCast(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 1, &frameCapture{})
	state.Spawn(live, 0, 0, 0, 0)

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), live, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	gcl.teleportLivePlayer(live, location.Location{X: 5000, Y: 5000, Z: 100}, 0)

	if controller.CastingNow() {
		t.Fatal("CastingNow() = true after a teleport, want cleared")
	}
}

// TestTeleportLivePlayerSendsActionFailedOnce pins the review-comment
// regression on PR #1227: teleportLivePlayer used to call both live.Stop()
// (which now reaches the cast controller, live_player.go:99-101) and
// live.Character.StopCast() (restartpoint.go) — the same *actorcast.Controller
// instance (castController wires live.Character's cast controller to
// live.cast, live_player.go:270), so an in-flight cast was stopped twice.
// Controller.stopInternal fires its stop-ack observer unconditionally on
// every call (controller.go:366-384), and clearLocked never resets it, so
// the second Stop() sent a second FrameActionFailed — a wire divergence
// from PlayerCast.stop()'s single, unconditional clientActionFailed()
// (PlayerCast.java:382-387).
func TestTeleportLivePlayerSendsActionFailedOnce(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), live, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	frames.frames = nil

	gcl.teleportLivePlayer(live, location.Location{X: 5000, Y: 5000, Z: 100}, 0)

	count := 0
	for _, f := range frames.frames {
		if f[0] == serverpackets.OpcodeActionFailed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ActionFailed frames after teleporting mid-cast = %d, want 1", count)
	}
}

// TestTeleportLivePlayerDoesNotAbortFusionChannel pins issue #1115: the
// reference has no teleport hook for fusion channels (only death, class
// change, logout, and party leave/expel/disperse — Player.java:2663,5847,6302;
// Party.java:202,428), relying instead on the existing 1s FusionChannelValid
// range+LOS recheck (FusionSkill.java:45-49) to catch an out-of-range
// teleport. Before this fix, teleportLivePlayer called abortFusionTargeting
// unconditionally, killing any channel targeting the teleporting player
// immediately, including a short in-range hop that Java would leave intact.
func TestTeleportLivePlayerDoesNotAbortFusionChannel(t *testing.T) {
	state := world.New()
	caster := newTestLivePlayer(t, 1, &frameCapture{})
	target := newTestLivePlayer(t, 2, &frameCapture{})
	state.Spawn(caster, 0, 0, 0, 0)
	state.Spawn(target, 100, 0, 0, 0)

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	controller := gcl.castController(caster)
	if _, err := controller.Start(time.Now(), caster, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	caster.setFusionTarget(target.ObjectID())

	// A short in-range hop stays within castRange + collision radii and LOS,
	// so the channel must survive it.
	gcl.teleportLivePlayer(target, location.Location{X: 150, Y: 0, Z: 0}, 0)

	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after target's in-range teleport, want fusion channel to survive (no reference teleport-abort hook)")
	}
	if !caster.fusesTarget(target.ObjectID()) {
		t.Fatal("fusesTarget() = false after target's in-range teleport, want fusion target unchanged")
	}
}

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

	gcl := &GameClientLink{world: state, geo: testGeo{}, restarts: townRestartTable(), playerConfig: PlayerConfig{RespawnRestoreHP: 0.7}, log: zerolog.Nop()}
	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{})

	if len(frames.frames) != 0 {
		t.Fatalf("frames sent for a living player = %d, want 0", len(frames.frames))
	}
}

func TestRestartLivePlayerStopsFakeDeathWithoutRevivingOrTeleporting(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	gcl := &GameClientLink{log: zerolog.Nop()}
	live.SetStanceBroadcaster(func(stance player.Stance) {
		if stance != player.StanceFakeDeathStop {
			return
		}
		x, y, z := live.Position()
		gcl.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameChangeWaitType(live.ObjectID(), serverpackets.WaitFakeDeathStop, location.Location{X: x, Y: y, Z: z})
		})
	})
	live.SetFakeDeathReviveBroadcaster(func() { gcl.broadcastLiveRevive(live) })

	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatalf("effect.New(FakeDeath): %v", err)
	}
	e.Effected = live.Character
	live.EffectList().Add(e)
	frames.frames = nil

	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{})

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeRevive}) {
		t.Fatalf("fake-death restart opcodes = %x, want ChangeWaitType, Revive", got)
	}
	if live.FakeDead() {
		t.Fatal("FakeDead() = true after restart request")
	}
	if live.Dead() {
		t.Fatal("Dead() = true after fake-death restart request")
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true after fake-death restart request")
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
	gcl := &GameClientLink{world: state, geo: testGeo{}, restarts: restarts, playerConfig: PlayerConfig{RespawnRestoreHP: 0.7}, log: zerolog.Nop()}
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

func TestCompleteLivePlayerTeleportRelocatesActiveSummonOnce(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 1, &frameCapture{})
	state.Spawn(live, 0, 0, 0, 0)
	observerFrames := &frameCapture{}
	observer := newTestLivePlayer(t, 2, observerFrames)
	state.Spawn(observer, 50, 0, 0, 0)
	active := summon.NewServitor(summon.ServitorConfig{ObjectID: 3, Owner: live, Level: 40})
	summon.SpawnBesideOwner(state, active, live, location.Location{})

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	destination := location.Location{X: 100, Y: 0, Z: 0}
	gcl.teleportLivePlayer(live, destination, 0)
	if got := summonPosition(active); got == destination {
		t.Fatalf("summon position before Appearing = %+v, want not yet relocated to %+v", got, destination)
	}
	observerFrames.frames = nil

	gcl.completeLivePlayerTeleport(live)
	if got := summonPosition(active); got != destination {
		t.Fatalf("summon position after Appearing = %+v, want %+v", got, destination)
	}
	if !containsTeleportFor(observerFrames.frames, active.ObjectID()) {
		t.Fatal("observer did not receive the active summon teleport frame")
	}

	active.SyncPosition(location.Location{})
	observerFrames.frames = nil
	gcl.completeLivePlayerTeleport(live)
	if got := summonPosition(active); got != (location.Location{}) {
		t.Fatalf("summon position after repeated Appearing = %+v, want unchanged", got)
	}
	if len(observerFrames.frames) != 0 {
		t.Fatalf("observer frames after repeated Appearing = %x, want none", observerFrames.frames)
	}
}

func summonPosition(actor *summon.Actor) location.Location {
	x, y, z := actor.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func containsTeleportFor(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if len(frame) >= 5 && frame[0] == serverpackets.OpcodeTeleportToLocation && int32(binary.LittleEndian.Uint32(frame[1:])) == objectID {
			return true
		}
	}
	return false
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
