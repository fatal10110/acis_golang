//go:build integration

package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestGameClientLinkSecondActionClickAttacksSelectedTarget is the
// regression test for the unresponsive-attack bug: the client attacks a mob
// by plain-clicking it twice (both clicks arrive as Action packets, not
// AttackRequest), and the second click must start the attack — in range it
// swings immediately, out of range it starts walking. Dropping that second
// click silently leaves the client's pending attack action unresolved, so
// the character walks up client-side, never swings, and stops responding
// to further input.
func TestGameClientLinkSecondActionClickAttacksSelectedTarget(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)
	live.Character.SetRollSource(func(int) int { return 0 })

	px, py, pz := live.Position()

	target := newTestHostileNPC(t, 3007)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })
	state.Spawn(target, px+30, py, pz, 0)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.Send(encodeAction(target.ObjectID(), origin, false))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("first Action opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("first Action second frame opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}
	c.Read() // StatusUpdate

	c.Send(encodeAction(target.ObjectID(), origin, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeAutoAttackStart {
		t.Fatalf("second Action opcode = %#x, want AutoAttackStart (%#x)", reply[0], serverpackets.OpcodeAutoAttackStart)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeAttack {
		t.Fatalf("second Action follow-up opcode = %#x, want Attack (%#x)", reply[0], serverpackets.OpcodeAttack)
	}
	r := wire.NewReader(reply[1:])
	if attackerID := r.ReadInt32(); attackerID != objID {
		t.Fatalf("Attack attacker id = %d, want %d", attackerID, objID)
	}
}

// TestGameClientLinkSecondActionClickWalksTowardDistantTarget covers the
// out-of-range half of the same regression: the second plain click on a far
// mob must answer with MoveToPawn (the walk into range), not silence.
func TestGameClientLinkSecondActionClickWalksTowardDistantTarget(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)
	px, py, pz := live.Position()

	target := newTestHostileNPC(t, 3008)
	state.Spawn(target, px+600, py, pz, 0)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.Send(encodeAction(target.ObjectID(), origin, false))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("first Action opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("first Action second frame opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}
	c.Read() // StatusUpdate

	c.Send(encodeAction(target.ObjectID(), origin, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("second Action on distant target opcode = %#x, want MoveToPawn (%#x)", reply[0], serverpackets.OpcodeMoveToPawn)
	}
}

func TestGameClientLinkAttackRequestFirstSelectsOnly(t *testing.T) {
	c, _, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	target := newTestHostileNPC(t, 3001)
	state.Spawn(target, 120, 20, 30, 0)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("visible target opcode = %#x, want NPCInfo (%#x)", reply[0], serverpackets.OpcodeNPCInfo)
	}

	origin := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeAttackRequest(target.ObjectID(), origin, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("first AttackRequest opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("first AttackRequest second frame opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}
	reply = c.Read()
	assertTargetHPStatus(t, reply, target.ObjectID(), target.MaxHP(), target.CurrentHP())

	c.Send(encodeRequestTargetCancel(1))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("RequestTargetCancel after first AttackRequest opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}
