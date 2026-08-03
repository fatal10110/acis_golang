package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestGameClientLinkActionBarStanceCommandsToggleStance covers the
// action-bar sit/stand and walk/run buttons, which arrive as action-use
// requests rather than the dedicated wait/move-type packets, and the
// release path for an action-bar command no handler claims yet: the client
// must get ActionFailed back, never silence.
func TestGameClientLinkActionBarStanceCommandsToggleStance(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// Walk/run button: a fresh character runs, so the first press walks and
	// the second runs again.
	c.send(encodeRequestActionUse(1, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("walk/run toggle opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objID)
	}
	if running := r.ReadInt32(); running != 0 {
		t.Fatalf("ChangeMoveType running = %d, want 0 after first toggle", running)
	}
	c.send(encodeRequestActionUse(1, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("run toggle opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if running := r.ReadInt32(); running != 1 {
		t.Fatalf("ChangeMoveType running = %d, want 1 after second toggle", running)
	}

	// Sit/stand button: a fresh character stands, so the first press sits
	// and the second stands back up.
	c.send(encodeRequestActionUse(0, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("sit toggle opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeWaitType object id = %d, want %d", got, objID)
	}
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitSitting) {
		t.Fatalf("ChangeWaitType type = %d, want sitting", waitType)
	}
	c.send(encodeRequestActionUse(0, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("stand toggle opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitStanding) {
		t.Fatalf("ChangeWaitType type = %d, want standing", waitType)
	}

	// An action-bar command nothing claims (private store sell) must
	// release the client with ActionFailed instead of silence.
	c.send(encodeRequestActionUse(10, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("unclaimed action opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

// TestRequestChangeWaitTypeRejectionSendsActionFailed pins Gap 1: a rejected
// sit/stand request (here, standing up while already standing, mirroring the
// reference's thinkStand guard) must release the client with ActionFailed
// instead of silently dropping the frame.
func TestRequestChangeWaitTypeRejectionSendsActionFailed(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	gcl := &GameClientLink{log: zerolog.Nop()}

	gcl.requestChangeWaitType(live, true)

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("rejected stand opcodes = %x, want ActionFailed", got)
	}
	if !live.Standing() {
		t.Fatal("live player stopped standing after a rejected stand request")
	}
}

// TestRequestChangeWaitTypeSitTargetsClaimableChair pins Gap 2: the sit key
// (RequestChangeWaitType with Stand == false) must bind a claimable targeted
// throne exactly like the click path does.
func TestRequestChangeWaitTypeSitTargetsClaimableChair(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	frames.frames = nil
	live.SetTargetTracked(chair)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.requestChangeWaitType(live, false)

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit}) {
		t.Fatalf("sit-key chair opcodes = %x, want ChangeWaitType, ChairSit", got)
	}
	if !chair.Busy() {
		t.Fatal("chair was not marked busy after sit key targeted it")
	}
}

// TestRequestChangeWaitTypeSitFallsBackWhenChairUnclaimable pins the
// reference's unconditional sitDown() ahead of its chair check: an
// unclaimable targeted chair (here, already busy) must not block the sit,
// it just leaves the player sitting on the ground instead of the throne.
func TestRequestChangeWaitTypeSitFallsBackWhenChairUnclaimable(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	chair.SetBusy(true)

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	frames.frames = nil
	live.SetTargetTracked(chair)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.requestChangeWaitType(live, false)

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType}) {
		t.Fatalf("sit-key opcodes with unclaimable chair = %x, want plain ChangeWaitType only", got)
	}
	if live.Standing() {
		t.Fatal("live player did not sit after an unclaimable chair target")
	}
}

func TestGameClientLinkStanceAndSocialPacketsInGame(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestChangeMoveType(false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("walk opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objID)
	}
	if running, swimming := r.ReadInt32(), r.ReadInt32(); running != 0 || swimming != 0 {
		t.Fatalf("ChangeMoveType flags = (%d,%d), want (0,0)", running, swimming)
	}

	c.send(encodeRequestChangeMoveType(true))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("run opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if running := r.ReadInt32(); running != 1 {
		t.Fatalf("ChangeMoveType running = %d, want 1", running)
	}

	c.send(encodeRequestChangeWaitType(false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("sit opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeWaitType object id = %d, want %d", got, objID)
	}
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitSitting) {
		t.Fatalf("ChangeWaitType type = %d, want sitting", waitType)
	}

	c.send(encodeRequestChangeWaitType(true))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("stand opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitStanding) {
		t.Fatalf("ChangeWaitType type = %d, want standing", waitType)
	}

	c.send(encodeRequestSocialAction(13))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSocialAction {
		t.Fatalf("social opcode = %#x, want SocialAction (%#x)", reply[0], serverpackets.OpcodeSocialAction)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("SocialAction object id = %d, want %d", got, objID)
	}
	if actionID := r.ReadInt32(); actionID != 13 {
		t.Fatalf("SocialAction action id = %d, want 13", actionID)
	}
}

func TestGameClientLinkAutoAttackStanceRefreshAndStop(t *testing.T) {
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	tracker := &attackStanceRecorder{}
	link := &GameClientLink{attackStance: tracker}

	link.startLiveAutoAttack(live)
	if len(tracker.actors) != 1 || tracker.actors[0].ObjectID() != live.ObjectID() {
		t.Fatalf("attack stance actors = %+v, want live player", tracker.actors)
	}
	if !live.InCombat() {
		t.Fatal("live player not marked in combat after AutoAttackStart")
	}
	if len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeAutoAttackStart {
		t.Fatalf("start frames = %x, want one AutoAttackStart", capture.frames)
	}

	link.startLiveAutoAttack(live)
	if len(tracker.actors) != 2 {
		t.Fatalf("attack stance refresh count = %d, want 2", len(tracker.actors))
	}
	if len(capture.frames) != 1 {
		t.Fatalf("second start emitted %d frames, want no duplicate AutoAttackStart", len(capture.frames)-1)
	}

	link.stopLiveAutoAttack(live)
	if live.InCombat() {
		t.Fatal("live player still marked in combat after AutoAttackStop")
	}
	if len(capture.frames) != 2 || capture.frames[1][0] != serverpackets.OpcodeAutoAttackStop {
		t.Fatalf("stop frames = %x, want AutoAttackStop", capture.frames)
	}

	link.stopLiveAutoAttack(live)
	if len(capture.frames) != 2 {
		t.Fatalf("second stop emitted %d frames, want no duplicate AutoAttackStop", len(capture.frames)-2)
	}
}
