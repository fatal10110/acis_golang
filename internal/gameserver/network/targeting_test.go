package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
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
	live.target = chair

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.handleTargetAction(live, chair.ObjectID(), true)

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

	target := newTestHostileNPC(t, 3000)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })
	state.Spawn(target, 120, 20, 30, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: 10, Y: 20, Z: 30}
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

func TestGameClientLinkAttackLiveTargetReusesController(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	attacker.stopAttack = gcl.stopLiveAutoAttack
	attacker.Character.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		gcl.broadcastAttack(attacker, snapshot)
	})
	target := newTestHostileNPC(t, 3002)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)
	attackerFrames.frames = nil

	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("first attackLiveTarget returned false")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeAutoAttackStart, serverpackets.OpcodeAttack}) {
		t.Fatalf("first attack opcodes = %x, want AutoAttackStart, Attack", got)
	}
	if attacker.attack == nil || !attacker.attack.AttackingNow() {
		t.Fatal("live player attack controller is not tracking the active attack")
	}

	attackerFrames.frames = nil
	if gcl.attackLiveTarget(attacker, target) {
		t.Fatal("second attackLiveTarget returned true while the first swing is active")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("second attack opcodes = %x, want ActionFailed only", got)
	}

	attacker.Stop()
	if attacker.attack.AttackingNow() {
		t.Fatal("live player Stop did not cancel the active attack controller")
	}
	if attacker.InCombat() {
		t.Fatal("live player Stop did not clear combat stance")
	}
}

func TestGameClientLinkAttackRequestFirstSelectsOnly(t *testing.T) {
	c, _, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	target := newTestHostileNPC(t, 3001)
	state.Spawn(target, 120, 20, 30, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: 10, Y: 20, Z: 30}
	c.send(encodeAttackRequest(target.ObjectID(), origin, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("first AttackRequest opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}
	reply = c.read()
	assertTargetHPStatus(t, reply, target.ObjectID(), target.MaxHP(), target.CurrentHP())

	c.send(encodeRequestTargetCancel(1))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("RequestTargetCancel after first AttackRequest opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
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

func TestGameClientLinkAttackBroadcastSendsToSelfAndObservers(t *testing.T) {
	state := world.New()
	link := &GameClientLink{world: state}
	attackerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	attackerFrames.frames = nil
	observerFrames.frames = nil

	link.broadcastAttack(attacker, attack.Snapshot{
		AttackerID: attacker.ObjectID(),
		X:          10,
		Y:          20,
		Z:          30,
		Hits:       []attack.SnapshotHit{{TargetID: observer.ObjectID(), Damage: 7}},
	})

	if len(attackerFrames.frames) != 1 || attackerFrames.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("attacker frames = %x, want one Attack", attackerFrames.frames)
	}
	if len(observerFrames.frames) != 1 || observerFrames.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("observer frames = %x, want one Attack", observerFrames.frames)
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
