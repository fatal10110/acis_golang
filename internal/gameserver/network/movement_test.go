package network

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// castingDef is a minimal long-hitTime active skill definition, long enough
// that Start leaves a real cast in flight for an actor-state abort test to
// interrupt.
var castingDef = modelskill.Definition{
	ID: 9, Level: 1, HitTime: 5000, StaticHitTime: true, StaticReuse: true,
}

// TestLiveMoveSpeedUsesSwimSpeedInWater pins PlayerStatus.getRealMoveSpeed:
// while inside a water zone, move speed switches to the template's swim
// speed regardless of the run/walk toggle, instead of the land run/walk
// speed.
func TestLiveMoveSpeedUsesSwimSpeedInWater(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	live.zoneActor = &liveZoneActor{live: live}

	live.SetRunning(true)
	if got := liveMoveSpeed(live); got != live.RunSpeed() {
		t.Fatalf("liveMoveSpeed() on land while running = %v, want RunSpeed() %v", got, live.RunSpeed())
	}

	live.zoneActor.ZoneFlags().Set(zone.FlagWater, true)
	if got := liveMoveSpeed(live); got != live.SwimSpeed() {
		t.Fatalf("liveMoveSpeed() in water = %v, want SwimSpeed() %v", got, live.SwimSpeed())
	}

	live.SetRunning(false)
	live.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 5}})
	if got := liveMoveSpeed(live); got != live.SwimSpeed() {
		t.Fatalf("liveMoveSpeed() in water while walking = %v, want SwimSpeed() %v (swim speed ignores run/walk)", got, live.SwimSpeed())
	}

	live.zoneActor.ZoneFlags().Set(zone.FlagWater, false)
	if got, want := liveMoveSpeed(live), live.WalkSpeed(); got != want {
		t.Fatalf("liveMoveSpeed() back on land while walking = %v, want stat-modified WalkSpeed %v", got, want)
	}
}

// TestChangeLiveMoveTypeReportsSwimmingInWaterZone pins ChangeMoveType.java:
// the swimming byte reflects Creature.isInWater() at the moment of the
// run/walk toggle, not a hardcoded land value.
func TestChangeLiveMoveTypeReportsSwimmingInWaterZone(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	live.zoneActor = &liveZoneActor{live: live}
	gcl := &GameClientLink{log: zerolog.Nop()}

	gcl.changeLiveMoveType(live, false)
	frames := capture.Frames()
	if len(frames) != 1 {
		t.Fatalf("frames captured = %d, want 1", len(frames))
	}
	r := wire.NewReader(frames[0][1:])
	r.ReadInt32() // objectID
	r.ReadInt32() // running
	if swimming := r.ReadInt32(); swimming != 0 {
		t.Fatalf("ChangeMoveType swimming on land = %d, want 0", swimming)
	}

	live.zoneActor.ZoneFlags().Set(zone.FlagWater, true)
	gcl.changeLiveMoveType(live, true)
	frames = capture.Frames()
	if len(frames) != 2 {
		t.Fatalf("frames captured = %d, want 2", len(frames))
	}
	r = wire.NewReader(frames[1][1:])
	r.ReadInt32()
	r.ReadInt32()
	if swimming := r.ReadInt32(); swimming != 1 {
		t.Fatalf("ChangeMoveType swimming in water = %d, want 1", swimming)
	}
}

// TestMoveLivePlayerStopsInFlightCast pins PlayerAI.onEvtCancel: a
// client-initiated walk cancels the AI's current intention, including an
// in-flight cast.
// TestMoveLivePlayerLeavesInFlightCastRunning pins PlayableAI.tryToMoveTo
// (PlayableAI.java:392-409): a walk request never touches getCast() — only
// RequestTargetCancel's Esc path (PlayerAI.onEvtCancel) can abort a cast.
// The PR under review (#1021) wrongly stopped the cast here, misattributing
// it to onEvtCancel; this pins the reference behavior instead.
func TestMoveLivePlayerLeavesInFlightCastRunning(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), live, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	gcl.moveLivePlayer(live, location.Location{X: 100})

	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after a client-initiated walk, want cast left running")
	}
}

// blockedTestGeo is a move.Geo double that rejects every straight-line move
// and finds no pathfinding alternative, matching the move package's own
// "no-progress fall-back" fixture: a fully geo-blocked destination.
type blockedTestGeo struct{}

func (blockedTestGeo) CanMove(int, int, int, int, int, int) bool { return false }
func (blockedTestGeo) Height(_, _, z int) int16                  { return int16(z) }
func (blockedTestGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (blockedTestGeo) Walkable(int, int, int) bool { return true }
func (blockedTestGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// newTestLivePlayerWithGeo builds a live player identical to
// newTestLivePlayer but wired to a caller-supplied move.Geo, so a test can
// exercise a geo-rejected route without touching the always-passable
// default.
func newTestLivePlayerWithGeo(t *testing.T, id int32, capture *testsupport.FrameCapture, geo move.Geo) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		CharLevel: 1,
		Location:  location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 80, MaxMP: 30, CurrentMP: 30})
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, testItemTemplates(), nil))
	ch.SetFrameSender(capture.Send)
	ch.SetBroadcastFrameSender(capture.Send)

	x, y, z := ch.Position()
	live, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, tmpl.RunSpeed, geo, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	moveCtl, err := move.NewController(ch.Move(), ch)
	if err != nil {
		t.Fatal(err)
	}
	attackCtl := attack.NewPlayer(ch)
	combat := ai.NewPlayerAttack(ch, moveCtl, attackCtl)
	moveCtl.SetArrived(combat.Think)
	attackCtl.SetFinished(combat.Think)

	return &livePlayer{Character: ch, template: tmpl, attack: attackCtl, move: moveCtl, combat: combat, visibilitySend: capture.Send}
}

// TestMoveLivePlayerSimulatesWalkServerSide pins the fix for #1168: the walk
// is driven through the move controller from the server-authoritative
// position, not the packet's claimed origin, so the broadcast MoveToLocation
// event always carries the server's own position as origin.
func TestMoveLivePlayerSimulatesWalkServerSide(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}

	var got move.Event
	events := 0
	live.Character.SetMoveBroadcaster(func(event move.Event) {
		events++
		got = event
	})

	origin := live.CurrentLocation()
	target := location.Location{X: origin.X + 8192, Y: origin.Y, Z: origin.Z}
	gcl.moveLivePlayer(live, target)

	if events != 1 {
		t.Fatalf("move broadcasts = %d, want 1", events)
	}
	if got.Origin != origin {
		t.Fatalf("broadcast origin = %+v, want server position %+v", got.Origin, origin)
	}
	if got.Destination != target {
		t.Fatalf("broadcast destination = %+v, want %+v", got.Destination, target)
	}
	if heading := live.CurrentHeading(); heading != origin.HeadingTo(target) {
		t.Fatalf("heading = %d, want %d (facing target from server position)", heading, origin.HeadingTo(target))
	}
}

// addLiveEffect installs a named effect on live's effect list, mirroring
// the npc package's addHostileEffect for player-side crowd-control tests.
func addLiveEffect(t *testing.T, live *livePlayer, name string) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = live
	live.EffectList().Add(e)
}

// liveFearFleeTarget satisfies the flee hook Fear's runtime needs to
// activate (fearAction requires an effect.Participant implementing
// FleeFrom) — production Player actors don't implement it yet (tracked
// under #117), so addLiveFearEffect substitutes this stub as Effected while
// still adding to live's own EffectList, exactly as addCharacterEffect does
// in player/character_cc_test.go.
type liveFearFleeTarget struct{}

func (liveFearFleeTarget) ObjectID() int32                                    { return 0 }
func (liveFearFleeTarget) Dead() bool                                         { return false }
func (liveFearFleeTarget) FleeFrom(effector effect.Participant, distance int) {}

func addLiveFearEffect(t *testing.T, live *livePlayer) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "Fear"})
	if err != nil {
		t.Fatalf("effect.New(Fear) error: %v", err)
	}
	e.Effected = liveFearFleeTarget{}
	live.EffectList().Add(e)
}

// TestMoveLivePlayerRejectsOutOfControl pins MoveBackwardToLocation.java:76's
// isOutOfControl() reject (Creature.java:652-655): a teleporting,
// immobile-until-attacked, stunned, sleeping, paralyzed, afraid, confused, or
// dead player's walk request is refused with ActionFailed and the server
// position never moves.
func TestMoveLivePlayerRejectsOutOfControl(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *livePlayer)
	}{
		{"teleporting", func(t *testing.T, live *livePlayer) {
			live.Character.SetTeleporting(true)
		}},
		{"immobile until attacked", func(t *testing.T, live *livePlayer) {
			addLiveEffect(t, live, "ImmobileUntilAttacked")
		}},
		{"stunned", func(t *testing.T, live *livePlayer) {
			addLiveEffect(t, live, "Stun")
		}},
		{"sleeping", func(t *testing.T, live *livePlayer) {
			addLiveEffect(t, live, "Sleep")
		}},
		{"paralyzed", func(t *testing.T, live *livePlayer) {
			addLiveEffect(t, live, "Paralyze")
		}},
		{"afraid", func(t *testing.T, live *livePlayer) {
			addLiveFearEffect(t, live)
		}},
		{"confused", func(t *testing.T, live *livePlayer) {
			addLiveEffect(t, live, "Confusion")
		}},
		{"dead", func(t *testing.T, live *livePlayer) {
			live.MarkDead()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, capture)
			gcl := &GameClientLink{log: zerolog.Nop()}
			tt.setup(t, live)

			origin := live.CurrentLocation()
			gcl.moveLivePlayer(live, location.Location{X: origin.X + 8192, Y: origin.Y, Z: origin.Z})

			opcodes := testsupport.FrameOpcodes(capture.Frames())
			if len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("frames sent = %x, want a single ActionFailed (%#x)", opcodes, serverpackets.OpcodeActionFailed)
			}
			if got := live.CurrentLocation(); got != origin {
				t.Fatalf("position after a rejected move = %+v, want unchanged %+v", got, origin)
			}
		})
	}
}

func TestMoveLivePlayerBroadcastsBlockedRouteAsZeroDistanceMove(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayerWithGeo(t, 1, capture, blockedTestGeo{})
	gcl := &GameClientLink{log: zerolog.Nop()}

	moveBroadcasts := 0
	var gotEvent move.Event
	live.Character.SetMoveBroadcaster(func(event move.Event) {
		moveBroadcasts++
		gotEvent = event
	})

	origin := live.CurrentLocation()
	gcl.moveLivePlayer(live, location.Location{X: origin.X + 500, Y: origin.Y, Z: origin.Z})

	if moveBroadcasts != 1 {
		t.Fatalf("move broadcasts on a blocked route = %d, want 1", moveBroadcasts)
	}
	opcodes := testsupport.FrameOpcodes(capture.Frames())
	if len(opcodes) != 0 {
		t.Fatalf("frames sent = %x, want none", opcodes)
	}
	if want := (move.Event{Origin: origin, Destination: origin, Speed: 120}); gotEvent != want {
		t.Fatalf("broadcast move = %+v, want %+v", gotEvent, want)
	}
	if got := live.move.Position(); got != origin {
		t.Fatalf("position before zero-distance arrival = %+v, want %+v", got, origin)
	}
	if got := live.CurrentHeading(); got != origin.HeadingTo(location.Location{X: origin.X + 500, Y: origin.Y, Z: origin.Z}) {
		t.Fatalf("heading after accepted zero-distance route = %d, want %d", got, origin.HeadingTo(location.Location{X: origin.X + 500, Y: origin.Y, Z: origin.Z}))
	}
}

// TestStopLivePlayerStopsSimulatedWalk pins CannotMoveAnymore's new
// semantics: it stops the server-simulated walk at wherever the simulation
// stands and never adopts client-reported coordinates (there are none to
// adopt — the handler no longer accepts any).
func TestStopLivePlayerStopsSimulatedWalk(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}

	origin := live.CurrentLocation()
	live.Character.SetMoveBroadcaster(func(move.Event) {})
	gcl.moveLivePlayer(live, location.Location{X: origin.X + 8192, Y: origin.Y, Z: origin.Z})

	stops := 0
	live.Character.SetStopBroadcaster(func() { stops++ })

	gcl.stopLivePlayer(live)
	if stops != 1 {
		t.Fatalf("stop broadcasts after stopping an in-flight walk = %d, want 1", stops)
	}
	if got := live.move.Position(); got != origin {
		t.Fatalf("position after stop = %+v, want the simulation's own position %+v (no ticks elapsed)", got, origin)
	}

	// A second stop with nothing in flight must not re-broadcast: matches
	// the reference's stop() being a no-op once already stopped.
	gcl.stopLivePlayer(live)
	if stops != 1 {
		t.Fatalf("stop broadcasts after a redundant stop = %d, want still 1", stops)
	}
}

// TestValidateLivePlayerPositionNeverAdoptsAValidReport pins ValidatePosition's
// new semantics: a report within the movement-speed threshold changes
// nothing server-side, and even a divergent report that does trigger a
// correction is never adopted as the new server position.
func TestValidateLivePlayerPositionNeverAdoptsAValidReport(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	gcl := &GameClientLink{log: zerolog.Nop()}
	origin := live.CurrentLocation()

	nearReport := location.Location{X: origin.X + 1, Y: origin.Y, Z: origin.Z}
	gcl.validateLivePlayerPosition(live, nearReport)
	if len(capture.Frames()) != 0 {
		t.Fatalf("frames sent for an in-threshold report = %d, want 0", len(capture.Frames()))
	}
	if got := live.CurrentLocation(); got != origin {
		t.Fatalf("position after an in-threshold report = %+v, want unchanged %+v", got, origin)
	}

	farReport := location.Location{X: origin.X + 8192, Y: origin.Y, Z: origin.Z}
	gcl.validateLivePlayerPosition(live, farReport)
	opcodes := testsupport.FrameOpcodes(capture.Frames())
	if len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("frames sent for an out-of-threshold report = %x, want a single ValidateLocation (%#x)", opcodes, serverpackets.OpcodeValidateLocation)
	}
	if got := live.CurrentLocation(); got != origin {
		t.Fatalf("position after an out-of-threshold report = %+v, want unchanged server position %+v (never adopted)", got, origin)
	}
}

// TestValidateLivePlayerPositionSkipsWhileTeleporting pins
// ValidatePosition.java:39, which returns before any correction while
// isTeleporting() — unlike the move/attack gates, it sends no ActionFailed
// either, matching the reference's silent early return (#1574).
func TestValidateLivePlayerPositionSkipsWhileTeleporting(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	gcl := &GameClientLink{log: zerolog.Nop()}
	live.Character.SetTeleporting(true)

	origin := live.CurrentLocation()
	farReport := location.Location{X: origin.X + 8192, Y: origin.Y, Z: origin.Z}
	gcl.validateLivePlayerPosition(live, farReport)

	if len(capture.Frames()) != 0 {
		t.Fatalf("frames sent while teleporting = %d, want 0", len(capture.Frames()))
	}
	if got := live.CurrentLocation(); got != origin {
		t.Fatalf("position while teleporting = %+v, want unchanged %+v", got, origin)
	}
}

// TestChangeLiveWaitTypeSitStopsInFlightCast pins the sit-down half of the
// reference's cast-abort surface: sitting down stops an in-flight cast,
// standing up does not.
// TestChangeLiveWaitTypeSitLeavesInFlightCastRunning pins Player.sitDown()
// (Player.java:1542-1565) and PlayerAI.thinkSit (PlayerAI.java:464-487):
// neither touches getCast() — a cast lands while seated in the reference.
// The PR under review (#1021) wrongly stopped the cast here; this pins the
// reference behavior instead.
func TestChangeLiveWaitTypeSitLeavesInFlightCastRunning(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), live, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if !gcl.changeLiveWaitType(live, false) {
		t.Fatal("changeLiveWaitType(sit) = false, want true")
	}
	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after sitting down, want cast left running")
	}
}

func TestMoveLivePlayerRelocatesWorldVisibility(t *testing.T) {
	state := world.New()
	movingFrames := &testsupport.FrameCapture{}
	watcherFrames := &testsupport.FrameCapture{}
	moving := newTestLivePlayer(t, 1, movingFrames)
	watcher := newTestLivePlayer(t, 2, watcherFrames)

	state.Spawn(moving, 0, 0, 0, 0)
	state.Spawn(watcher, 8192, 0, 0, 0)
	if world.Knows(moving, watcher) {
		t.Fatal("players unexpectedly know each other before movement")
	}

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.updateLivePlayerPosition(moving, location.Location{X: 6144, Y: 0, Z: 0}, 123)

	if !world.Knows(moving, watcher) {
		t.Fatal("players do not know each other after movement into visibility range")
	}
	if got := testsupport.FrameOpcodes(movingFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodeCharInfo}) {
		t.Fatalf("moving player opcodes = %x, want CharInfo", got)
	}
	if got := testsupport.FrameOpcodes(watcherFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodeCharInfo}) {
		t.Fatalf("watcher opcodes = %x, want CharInfo", got)
	}
}

func TestBroadcastLiveDieSendsDieToOwnSessionAndObservers(t *testing.T) {
	state := world.New()
	victimFrames := &testsupport.FrameCapture{}
	observerFrames := &testsupport.FrameCapture{}
	victim := newTestLivePlayer(t, 1, victimFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	victim.AccessLevel = 7

	state.Spawn(victim, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	testsupport.ResetCapture(victimFrames)
	testsupport.ResetCapture(observerFrames)

	adminData, err := admin.NewData([]admin.AccessLevel{{Level: 7, AllowFixedRes: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gcl := &GameClientLink{world: state, admin: adminData, log: zerolog.Nop()}
	gcl.broadcastLiveDie(victim)

	if got := testsupport.FrameOpcodes(victimFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodeDie}) {
		t.Fatalf("victim opcodes = %x, want Die", got)
	}
	if got := testsupport.FrameOpcodes(observerFrames.Frames()); string(got) != string([]byte{serverpackets.OpcodeDie}) {
		t.Fatalf("observer opcodes = %x, want Die", got)
	}
	for _, frames := range [][][]byte{victimFrames.Frames(), observerFrames.Frames()} {
		if got := binary.LittleEndian.Uint32(frames[0][25:29]); got != 1 {
			t.Fatalf("Die fixed-res field = %d, want 1", got)
		}
	}
}

func TestBroadcastLiveFrameReleasesKnownBufferBeforeDelivery(t *testing.T) {
	state := world.New()
	self := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	observer := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	link := &GameClientLink{world: state, log: zerolog.Nop()}
	var nested atomic.Bool
	observer.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		if nested.CompareAndSwap(false, true) {
			link.broadcastLiveFrame(self, func() wire.Frame {
				return serverpackets.FrameRevive(self.ObjectID())
			})
		}
		return true
	})

	done := make(chan struct{})
	go func() {
		link.broadcastLiveFrame(self, func() wire.Frame {
			return serverpackets.FrameRevive(self.ObjectID())
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast held KnownBuffer while delivering a frame")
	}
}

func TestBroadcastLiveFrameBuildsOnceForAllRecipients(t *testing.T) {
	state := world.New()
	selfFrames := &testsupport.FrameCapture{}
	observerFrames := &testsupport.FrameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	testsupport.ResetCapture(selfFrames)
	testsupport.ResetCapture(observerFrames)

	builds := 0
	(&GameClientLink{world: state, log: zerolog.Nop()}).broadcastLiveFrame(self, func() wire.Frame {
		builds++
		return serverpackets.FrameRevive(self.ObjectID())
	})

	if builds != 1 {
		t.Fatalf("frame builds = %d, want 1", builds)
	}
	if len(selfFrames.Frames()) != 1 || len(observerFrames.Frames()) != 1 {
		t.Fatalf("received frames = (%d, %d), want (1, 1)", len(selfFrames.Frames()), len(observerFrames.Frames()))
	}
	if !bytes.Equal(selfFrames.Frames()[0], observerFrames.Frames()[0]) {
		t.Fatalf("recipient frames differ: self %x observer %x", selfFrames.Frames()[0], observerFrames.Frames()[0])
	}
}

func TestBroadcastLiveFrameGivesRecipientsIndependentFrames(t *testing.T) {
	state := world.New()
	self := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	observer := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
	var selfFrame, observerFrame wire.Frame
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		selfFrame = frame
		return true
	})
	self.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
		selfFrame = frame
		return true
	})
	observer.Character.SetFrameSender(func(frame wire.Frame) bool {
		observerFrame = frame
		return true
	})
	observer.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
		observerFrame = frame
		return true
	})
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)

	(&GameClientLink{world: state, log: zerolog.Nop()}).broadcastLiveFrame(self, func() wire.Frame {
		return serverpackets.FrameRevive(self.ObjectID())
	})
	defer selfFrame.Release()
	defer observerFrame.Release()

	if len(selfFrame.Bytes()) <= wire.FrameHeaderSize || len(observerFrame.Bytes()) <= wire.FrameHeaderSize {
		t.Fatal("recipients did not receive frames")
	}
	observerPayload := observerFrame.Bytes()[wire.FrameHeaderSize]
	selfFrame.Bytes()[wire.FrameHeaderSize] ^= 0xff
	if observerFrame.Bytes()[wire.FrameHeaderSize] != observerPayload {
		t.Fatal("mutating one recipient frame changed another recipient frame")
	}
}

func TestBroadcastFrameBuildsOnceAndCopiesForRecipients(t *testing.T) {
	selfFrames := &testsupport.FrameCapture{}
	observerFrames := &testsupport.FrameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	builds := 0
	broadcastFrame(func() wire.Frame {
		builds++
		return serverpackets.FrameStatusUpdate(self.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: 100},
			{Type: serverpackets.StatusCurrentHP, Value: 75},
		})
	}, func(send func(frameReceiver)) {
		send(self)
		send(observer)
	})

	if builds != 1 {
		t.Fatalf("frame builds = %d, want 1", builds)
	}
	if len(selfFrames.Frames()) != 1 || len(observerFrames.Frames()) != 1 {
		t.Fatalf("recipient frame counts = %d, %d; want 1, 1", len(selfFrames.Frames()), len(observerFrames.Frames()))
	}
	if !bytes.Equal(selfFrames.Frames()[0], observerFrames.Frames()[0]) {
		t.Fatalf("recipient frames differ: self %x observer %x", selfFrames.Frames()[0], observerFrames.Frames()[0])
	}
}

func TestBroadcastFrameSkipsBuildWithoutRecipients(t *testing.T) {
	builds := 0
	broadcastFrame(func() wire.Frame {
		builds++
		return serverpackets.FrameRevive(1)
	}, func(func(frameReceiver)) {})
	if builds != 0 {
		t.Fatalf("frame builds = %d, want 0", builds)
	}
}

func BenchmarkBroadcastLiveFrameKnownObservers(b *testing.B) {
	// These senders release synchronously; production queues may retain many frames.
	state := world.New()
	self := newTestLivePlayer(b, 1, &testsupport.FrameCapture{})
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	self.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	state.Spawn(self, 0, 0, 0, 0)
	for i := 0; i < 50; i++ {
		observer := newTestLivePlayer(b, int32(i+2), &testsupport.FrameCapture{})
		observer.Character.SetFrameSender(func(frame wire.Frame) bool {
			frame.Release()
			return true
		})
		observer.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
			frame.Release()
			return true
		})
		state.Spawn(observer, i+100, 0, 0, 0)
	}

	link := &GameClientLink{world: state, log: zerolog.Nop()}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		link.broadcastLiveFrame(self, func() wire.Frame {
			return serverpackets.FrameRevive(self.ObjectID())
		})
	}
}

func BenchmarkBroadcastCharacterInfoKnownObservers(b *testing.B) {
	// These senders release synchronously; production queues may retain many frames.
	state := world.New()
	self := newTestLivePlayer(b, 1, &testsupport.FrameCapture{})
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	self.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	state.Spawn(self, 0, 0, 0, 0)
	for i := 0; i < 50; i++ {
		observer := newTestLivePlayer(b, int32(i+2), &testsupport.FrameCapture{})
		observer.Character.SetFrameSender(func(frame wire.Frame) bool {
			frame.Release()
			return true
		})
		observer.Character.SetBroadcastFrameSender(func(frame wire.Frame) bool {
			frame.Release()
			return true
		})
		state.Spawn(observer, i+100, 0, 0, 0)
	}

	link := &GameClientLink{world: state, log: zerolog.Nop()}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		link.broadcastCharacterInfo(self)
	}
}

// TestUpdateLivePlayerPositionReseedsCreatureMove is the regression test for
// the "first chase after a client walk computes its route from a stale
// seed" review finding: updateLivePlayerPosition must reseed the player's
// own CreatureMove, not just world.Presence, or a chase started right after
// a client-reported walk measures distance/duration from the old spot.
func TestUpdateLivePlayerPositionReseedsCreatureMove(t *testing.T) {
	state := world.New()
	moving := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	state.Spawn(moving, 0, 0, 0, 0)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	newPos := location.Location{X: 500, Y: 0, Z: 0}
	gcl.updateLivePlayerPosition(moving, newPos, 0)

	if got := moving.move.Position(); got != newPos {
		t.Fatalf("CreatureMove position after updateLivePlayerPosition = %+v, want %+v", got, newPos)
	}
}

func TestGameClientLinkWireSafeMovementAndRefreshPacketsInGame(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// spawn is the new character's actual server-authoritative position
	// (matching TestGameClientLinkLogoutLeavesWorld's default spawn). The
	// walk target is deliberately far enough away that, since this test
	// harness never wires a PositionUpdates ticker, the simulated walk
	// never arrives during the test — position stays pinned at spawn for
	// every check below, exactly like the reference never adopting a
	// client-reported position mid-walk.
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 46160, Y: 41237, Z: -3534}
	claimedOrigin := location.Location{X: 46117, Y: 41247, Z: -3532}
	walkHeading := spawn.HeadingTo(target)
	c.Send(encodeMoveBackwardToLocation(target, claimedOrigin, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("move reply opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", got, objID)
	}
	gotTarget := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotTarget != target {
		t.Fatalf("MoveToLocation target = %+v, want %+v", gotTarget, target)
	}
	gotOrigin := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotOrigin != spawn {
		t.Fatalf("MoveToLocation origin = %+v, want server position %+v (not the client's claimed origin %+v)", gotOrigin, spawn, claimedOrigin)
	}
	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	positioned, ok := obj.(interface{ Position() (int, int, int) })
	if !ok {
		t.Fatalf("world.Player(%d) has no Position method", objID)
	}
	x, y, z := positioned.Position()
	if x != spawn.X || y != spawn.Y || z != spawn.Z {
		t.Fatalf("player position after MoveBackwardToLocation = (%d,%d,%d), want spawn (%d,%d,%d) (the walk is simulated, not instantaneous)", x, y, z, spawn.X, spawn.Y, spawn.Z)
	}

	// A ValidatePosition report close to the server's actual (still-spawn)
	// position is within a second's travel and must not be adopted or
	// answered.
	nearClientPosition := location.Location{X: spawn.X + 1, Y: spawn.Y, Z: spawn.Z}
	c.Send(encodeValidatePosition(nearClientPosition, 32768))
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("item refresh opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
	x, y, z = positioned.Position()
	if x != spawn.X || y != spawn.Y || z != spawn.Z {
		t.Fatalf("player position after in-threshold ValidatePosition = (%d,%d,%d), want unchanged spawn (%d,%d,%d)", x, y, z, spawn.X, spawn.Y, spawn.Z)
	}

	// A report far from the server's actual position exceeds the
	// divergence threshold: the server corrects the client back to its
	// own (still-spawn) position, but never adopts the client's report.
	farClientPosition := location.Location{X: target.X + 500, Y: target.Y, Z: target.Z}
	c.Send(encodeValidatePosition(farClientPosition, 32768))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("desync correction opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ValidateLocation object id = %d, want %d", got, objID)
	}
	gotCorrection := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotCorrection != spawn {
		t.Fatalf("ValidateLocation location = %+v, want server position %+v", gotCorrection, spawn)
	}
	if heading := r.ReadInt32(); heading != int32(walkHeading) {
		t.Fatalf("ValidateLocation heading = %d, want walk heading %d", heading, walkHeading)
	}
	x, y, z = positioned.Position()
	if x != spawn.X || y != spawn.Y || z != spawn.Z {
		t.Fatalf("player position after desync ValidatePosition = (%d,%d,%d), want unchanged server position (%d,%d,%d)", x, y, z, spawn.X, spawn.Y, spawn.Z)
	}

	// CannotMoveAnymore is a stop report, not a position report: the
	// client-claimed stop coordinates and heading below must be discarded
	// entirely, and StopMove must carry the server's own (still-spawn,
	// still-mid-walk) position and heading instead.
	claimedStopAt := location.Location{X: 46155, Y: 41240, Z: -3534}
	c.Send(encodeCannotMoveAnymore(claimedStopAt, 12345))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeStopMove {
		t.Fatalf("stop reply opcode = %#x, want StopMove (%#x)", reply[0], serverpackets.OpcodeStopMove)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("StopMove object id = %d, want %d", got, objID)
	}
	gotStoppedAt := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotStoppedAt != spawn {
		t.Fatalf("StopMove location = %+v, want server position %+v (not the client's claimed stop point %+v)", gotStoppedAt, spawn, claimedStopAt)
	}
	if heading := r.ReadInt32(); heading != int32(walkHeading) {
		t.Fatalf("StopMove heading = %d, want walk heading %d (not the client's claimed heading 12345)", heading, walkHeading)
	}
	x, y, z = positioned.Position()
	if x != spawn.X || y != spawn.Y || z != spawn.Z {
		t.Fatalf("player position after CannotMoveAnymore = (%d,%d,%d), want unchanged server position (%d,%d,%d)", x, y, z, spawn.X, spawn.Y, spawn.Z)
	}

	c.Send(encodeStartRotating(32768, 1))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeStartRotation {
		t.Fatalf("start rotation opcode = %#x, want StartRotation (%#x)", reply[0], serverpackets.OpcodeStartRotation)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("StartRotation object id = %d, want %d", got, objID)
	}
	if degree, side, speed := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); degree != 32768 || side != 1 || speed != 0 {
		t.Fatalf("StartRotation fields = (%d,%d,%d), want (32768,1,0)", degree, side, speed)
	}

	c.Send(encodeFinishRotating(22222, 1))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeStopRotation {
		t.Fatalf("stop rotation opcode = %#x, want StopRotation (%#x)", reply[0], serverpackets.OpcodeStopRotation)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("StopRotation object id = %d, want %d", got, objID)
	}
	wantLowDegree := uint8(22222 & 0xff)
	if degree, speed, lowDegree := r.ReadInt32(), r.ReadInt32(), r.ReadUint8(); degree != 22222 || speed != 0 || lowDegree != wantLowDegree {
		t.Fatalf("StopRotation fields = (%d,%d,%d), want (22222,0,%d)", degree, speed, lowDegree, wantLowDegree)
	}
	if heading := obj.(*livePlayer).Character.CurrentHeading(); heading != 22222 {
		t.Fatalf("live player heading = %d, want 22222", heading)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("skill refresh opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}

	for _, opcode := range []byte{
		clientpackets.OpcodeSendWarehouseDeposit,
		clientpackets.OpcodeRequestQuestListInGame,
		clientpackets.OpcodeRequestPackageItemList,
		clientpackets.OpcodeGameGuardReply,
		clientpackets.OpcodeRequestShowMiniMap,
	} {
		c.Send(encodeSingleOpcode(opcode))
	}
	// DlgAnswer is wired (unlike the still-unwired opcodes above), so it
	// needs a well-formed body; a messageId with no pending dialog behind
	// it is still a no-op, matching DlgAnswer.runImpl's unmatched-id
	// fall-through (DlgAnswer.java:20-37).
	c.Send(encodeDlgAnswer(0, 0, 0))
	c.Send(encodeRequestManorList())
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("post-stub opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
}
