package network

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// castingDef is a minimal long-hitTime active skill definition, long enough
// that Start leaves a real cast in flight for an actor-state abort test to
// interrupt.
var castingDef = modelskill.Definition{
	ID: 9, Level: 1, HitTime: 5000, StaticHitTime: true, StaticReuse: true,
}

// TestMoveLivePlayerStopsInFlightCast pins PlayerAI.onEvtCancel: a
// client-initiated walk cancels the AI's current intention, including an
// in-flight cast.
func TestMoveLivePlayerStopsInFlightCast(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), skillCastObject(live), castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	gcl.moveLivePlayer(live, live.CurrentLocation(), location.Location{X: 100})

	if controller.CastingNow() {
		t.Fatal("CastingNow() = true after a client-initiated walk, want cleared")
	}
}

// TestChangeLiveWaitTypeSitStopsInFlightCast pins the sit-down half of the
// reference's cast-abort surface: sitting down stops an in-flight cast,
// standing up does not.
func TestChangeLiveWaitTypeSitStopsInFlightCast(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), skillCastObject(live), castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if !gcl.changeLiveWaitType(live, false) {
		t.Fatal("changeLiveWaitType(sit) = false, want true")
	}
	if controller.CastingNow() {
		t.Fatal("CastingNow() = true after sitting down, want cleared")
	}
}

func TestMoveLivePlayerRelocatesWorldVisibility(t *testing.T) {
	state := world.New()
	movingFrames := &frameCapture{}
	watcherFrames := &frameCapture{}
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
	if got := frameOpcodes(movingFrames.frames); string(got) != string([]byte{serverpackets.OpcodeCharInfo}) {
		t.Fatalf("moving player opcodes = %x, want CharInfo", got)
	}
	if got := frameOpcodes(watcherFrames.frames); string(got) != string([]byte{serverpackets.OpcodeCharInfo}) {
		t.Fatalf("watcher opcodes = %x, want CharInfo", got)
	}
}

func TestBroadcastLiveDieSendsDieToOwnSessionAndObservers(t *testing.T) {
	state := world.New()
	victimFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	victim := newTestLivePlayer(t, 1, victimFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(victim, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	victimFrames.frames = nil
	observerFrames.frames = nil

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.broadcastLiveDie(victim)

	if got := frameOpcodes(victimFrames.frames); string(got) != string([]byte{serverpackets.OpcodeDie}) {
		t.Fatalf("victim opcodes = %x, want Die", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeDie}) {
		t.Fatalf("observer opcodes = %x, want Die", got)
	}
}

func TestBroadcastLiveFrameBuildsOnceForAllRecipients(t *testing.T) {
	state := world.New()
	selfFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	selfFrames.frames = nil
	observerFrames.frames = nil

	builds := 0
	(&GameClientLink{world: state, log: zerolog.Nop()}).broadcastLiveFrame(self, func() wire.Frame {
		builds++
		return serverpackets.FrameRevive(self.ObjectID())
	})

	if builds != 1 {
		t.Fatalf("frame builds = %d, want 1", builds)
	}
	if len(selfFrames.frames) != 1 || len(observerFrames.frames) != 1 {
		t.Fatalf("received frames = (%d, %d), want (1, 1)", len(selfFrames.frames), len(observerFrames.frames))
	}
	if !bytes.Equal(selfFrames.frames[0], observerFrames.frames[0]) {
		t.Fatalf("recipient frames differ: self %x observer %x", selfFrames.frames[0], observerFrames.frames[0])
	}
}

func TestBroadcastLiveFrameGivesRecipientsIndependentFrames(t *testing.T) {
	state := world.New()
	self := newTestLivePlayer(t, 1, &frameCapture{})
	observer := newTestLivePlayer(t, 2, &frameCapture{})
	var selfFrame, observerFrame wire.Frame
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		selfFrame = frame
		return true
	})
	observer.Character.SetFrameSender(func(frame wire.Frame) bool {
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
	selfFrames := &frameCapture{}
	observerFrames := &frameCapture{}
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
	if len(selfFrames.frames) != 1 || len(observerFrames.frames) != 1 {
		t.Fatalf("recipient frame counts = %d, %d; want 1, 1", len(selfFrames.frames), len(observerFrames.frames))
	}
	if !bytes.Equal(selfFrames.frames[0], observerFrames.frames[0]) {
		t.Fatalf("recipient frames differ: self %x observer %x", selfFrames.frames[0], observerFrames.frames[0])
	}
}

func BenchmarkBroadcastLiveFrameKnownObservers(b *testing.B) {
	state := world.New()
	self := newTestLivePlayer(b, 1, &frameCapture{})
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	state.Spawn(self, 0, 0, 0, 0)
	for i := 0; i < 50; i++ {
		observer := newTestLivePlayer(b, int32(i+2), &frameCapture{})
		observer.Character.SetFrameSender(func(frame wire.Frame) bool {
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
	state := world.New()
	self := newTestLivePlayer(b, 1, &frameCapture{})
	self.Character.SetFrameSender(func(frame wire.Frame) bool {
		frame.Release()
		return true
	})
	state.Spawn(self, 0, 0, 0, 0)
	for i := 0; i < 50; i++ {
		observer := newTestLivePlayer(b, int32(i+2), &frameCapture{})
		observer.Character.SetFrameSender(func(frame wire.Frame) bool {
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
	moving := newTestLivePlayer(t, 1, &frameCapture{})
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

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	target := location.Location{X: 46160, Y: 41237, Z: -3534}
	origin := location.Location{X: 46117, Y: 41247, Z: -3532}
	c.send(encodeMoveBackwardToLocation(target, origin, 1))
	reply := c.read()
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
	if gotOrigin != origin {
		t.Fatalf("MoveToLocation origin = %+v, want %+v", gotOrigin, origin)
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
	if x != origin.X || y != origin.Y || z != origin.Z {
		t.Fatalf("player position after MoveBackwardToLocation = (%d,%d,%d), want origin (%d,%d,%d)", x, y, z, origin.X, origin.Y, origin.Z)
	}

	c.send(encodeValidatePosition(target, 32768))
	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("item refresh opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
	x, y, z = positioned.Position()
	if x != target.X || y != target.Y || z != target.Z {
		t.Fatalf("player position after ValidatePosition = (%d,%d,%d), want (%d,%d,%d)", x, y, z, target.X, target.Y, target.Z)
	}

	farClientPosition := location.Location{X: target.X + 500, Y: target.Y, Z: target.Z}
	c.send(encodeValidatePosition(farClientPosition, 32768))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("desync correction opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ValidateLocation object id = %d, want %d", got, objID)
	}
	gotCorrection := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotCorrection != target {
		t.Fatalf("ValidateLocation location = %+v, want server position %+v", gotCorrection, target)
	}
	if heading := r.ReadInt32(); heading != 32768 {
		t.Fatalf("ValidateLocation heading = %d, want 32768", heading)
	}
	x, y, z = positioned.Position()
	if x != target.X || y != target.Y || z != target.Z {
		t.Fatalf("player position after desync ValidatePosition = (%d,%d,%d), want server position (%d,%d,%d)", x, y, z, target.X, target.Y, target.Z)
	}

	stoppedAt := location.Location{X: 46155, Y: 41240, Z: -3534}
	c.send(encodeCannotMoveAnymore(stoppedAt, 12345))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeStopMove {
		t.Fatalf("stop reply opcode = %#x, want StopMove (%#x)", reply[0], serverpackets.OpcodeStopMove)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("StopMove object id = %d, want %d", got, objID)
	}
	gotStoppedAt := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotStoppedAt != stoppedAt {
		t.Fatalf("StopMove location = %+v, want %+v", gotStoppedAt, stoppedAt)
	}
	if heading := r.ReadInt32(); heading != 12345 {
		t.Fatalf("StopMove heading = %d, want 12345", heading)
	}
	x, y, z = positioned.Position()
	if x != stoppedAt.X || y != stoppedAt.Y || z != stoppedAt.Z {
		t.Fatalf("player position after CannotMoveAnymore = (%d,%d,%d), want (%d,%d,%d)", x, y, z, stoppedAt.X, stoppedAt.Y, stoppedAt.Z)
	}

	c.send(encodeStartRotating(32768, 1))
	reply = c.read()
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

	c.send(encodeFinishRotating(22222, 1))
	reply = c.read()
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

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("skill refresh opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}

	for _, opcode := range []byte{
		clientpackets.OpcodeSendWarehouseDeposit,
		clientpackets.OpcodeRequestQuestListInGame,
		clientpackets.OpcodeRequestPackageItemList,
		clientpackets.OpcodeDlgAnswer,
		clientpackets.OpcodeGameGuardReply,
		clientpackets.OpcodeRequestShowMiniMap,
	} {
		c.send(encodeSingleOpcode(opcode))
	}
	c.send(encodeRequestManorList())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("post-stub opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
}
