package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestGameClientLinkAttackBroadcastsTargetHPStatusOnEveryHit is the
// regression test for the reported bug: hitting an NPC does eventually kill
// it, but nothing shows the HP bar dropping along the way — TakeDamage
// applied the damage and ran the death check, but never told any client the
// target's HP had changed, so the only visible sign of a hit landing was
// the target's corpse.
func TestGameClientLinkAttackBroadcastsTargetHPStatusOnEveryHit(t *testing.T) {
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

	target := newTestHostileNPC(t, 3010)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.Instance.Template.HPMax = 1000
	target.SetRollSource(func(int) int { return 0 })
	// A hostile NPC only broadcasts through a world it was registered
	// into, matching how the production spawn manager wires it — see
	// data/manager/npcs.go's newLiveHostile.
	target.SetWorld(state)
	state.Spawn(target, px+30, py, pz, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.send(encodeAction(target.ObjectID(), origin, false))
	c.read() // MyTargetSelected
	c.read() // StatusUpdate (selection snapshot, still full HP)

	c.send(encodeAttackRequest(target.ObjectID(), origin, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeAutoAttackStart {
		t.Fatalf("AttackRequest first opcode = %#x, want AutoAttackStart (%#x)", reply[0], serverpackets.OpcodeAutoAttackStart)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeAttack {
		t.Fatalf("AttackRequest opcode = %#x, want Attack (%#x)", reply[0], serverpackets.OpcodeAttack)
	}

	reply := c.read()
	if reply[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("post-hit opcode = %#x, want StatusUpdate (%#x) — the hit landed with no visible HP change", reply[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != target.ObjectID() {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, target.ObjectID())
	}
	r.ReadInt32() // attribute count
	r.ReadInt32() // MAX_HP type
	r.ReadInt32() // MAX_HP value
	if typ := r.ReadInt32(); typ != int32(serverpackets.StatusCurrentHP) {
		t.Fatalf("StatusUpdate second attribute type = %d, want CUR_HP (%d)", typ, serverpackets.StatusCurrentHP)
	}
	if curHP := r.ReadInt32(); curHP >= int32(target.MaxHP()) {
		t.Fatalf("StatusUpdate current HP = %d, want less than max HP %d after a landed hit", curHP, target.MaxHP())
	}
	if target.CurrentHP() >= target.MaxHP() {
		t.Fatalf("target.CurrentHP() = %d, want less than max HP %d after a landed hit", target.CurrentHP(), target.MaxHP())
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
