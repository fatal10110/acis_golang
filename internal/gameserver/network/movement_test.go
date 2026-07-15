package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

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
