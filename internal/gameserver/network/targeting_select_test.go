package network

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestSelectAndClearLiveTargetSendsTargetPackets(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	target := newTestHostileNPC(t, 3)

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	state.Spawn(target, 150, 0, 0, 0)
	attackerFrames.frames = nil
	observerFrames.frames = nil

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if !gcl.selectLiveTarget(attacker, target) {
		t.Fatal("selectLiveTarget returned false")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeMyTargetSelected, serverpackets.OpcodeStatusUpdate}) {
		t.Fatalf("attacker select opcodes = %x, want MyTargetSelected, StatusUpdate", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeTargetSelected}) {
		t.Fatalf("observer select opcodes = %x, want TargetSelected", got)
	}

	attackerFrames.frames = nil
	observerFrames.frames = nil
	gcl.clearLiveTarget(attacker)
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("attacker clear opcodes = %x, want ActionFailed", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeTargetUnselected}) {
		t.Fatalf("observer clear opcodes = %x, want TargetUnselected", got)
	}
}

func TestGameClientLinkActionSitsOnSelectedChairStaticObject(t *testing.T) {
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
	gcl.handleTargetAction(context.Background(), live, chair.ObjectID(), true, false)

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit}) {
		t.Fatalf("chair action opcodes = %x, want ChangeWaitType, ChairSit", got)
	}
	if live.Standing() {
		t.Fatal("live player remained standing after chair action")
	}
	if !chair.Busy() {
		t.Fatal("chair was not marked busy")
	}

	r := wire.NewReader(frames.frames[1][1:])
	if got := r.ReadInt32(); got != live.ObjectID() {
		t.Fatalf("ChairSit player id = %d, want %d", got, live.ObjectID())
	}
	if got := r.ReadInt32(); got != int32(chair.StaticObjectID()) {
		t.Fatalf("ChairSit static id = %d, want %d", got, chair.StaticObjectID())
	}

	frames.frames = nil
	gcl.changeLiveWaitType(live, true)
	if chair.Busy() {
		t.Fatal("chair stayed busy after standing")
	}
	if !live.Standing() {
		t.Fatal("live player did not stand after stand request")
	}
	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType}) {
		t.Fatalf("stand opcodes = %x, want ChangeWaitType", got)
	}
}

func TestGameClientLinkResolveTargetFallsBackToPlayerRegistry(t *testing.T) {
	state := world.New()
	targetFrames := &frameCapture{}
	target := newTestLivePlayer(t, 42, targetFrames)
	state.AddPlayer(target)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if got := gcl.resolveTarget(target.ObjectID()); got != target {
		t.Fatalf("resolveTarget(player) = %v, want player registry target", got)
	}
}

func TestGameClientLinkActionAttackAndTargetCancel(t *testing.T) {
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

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)
	live.Character.SetRollSource(func(int) int { return 0 })

	px, py, pz := live.Position()

	target := newTestHostileNPC(t, 3000)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })
	// Spawned well within melee range so this exercises the immediate-attack
	// path; out-of-range approach is covered separately.
	state.Spawn(target, px+30, py, pz, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.send(encodeAction(target.ObjectID(), origin, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("Action first opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}
	reply = c.read()
	assertTargetHPStatus(t, reply, target.ObjectID(), target.MaxHP(), target.CurrentHP())

	c.send(encodeAttackRequest(target.ObjectID(), origin, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeAutoAttackStart {
		t.Fatalf("AttackRequest first opcode = %#x, want AutoAttackStart (%#x)", reply[0], serverpackets.OpcodeAutoAttackStart)
	}
	r := wire.NewReader(reply[1:])
	if attackerID := r.ReadInt32(); attackerID != objID {
		t.Fatalf("AutoAttackStart object id = %d, want %d", attackerID, objID)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeAttack {
		t.Fatalf("AttackRequest opcode = %#x, want Attack (%#x)", reply[0], serverpackets.OpcodeAttack)
	}
	r = wire.NewReader(reply[1:])
	if attackerID := r.ReadInt32(); attackerID != objID {
		t.Fatalf("Attack attacker id = %d, want %d", attackerID, objID)
	}

	c.send(encodeRequestTargetCancel(1))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("RequestTargetCancel opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}
