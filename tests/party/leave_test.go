package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func threeMemberParty(t *testing.T) *group {
	t.Helper()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	return g
}

// TestWithdrawFromParty pins RequestWithdrawParty with three members: the
// leaver is told it left and its window cleared; the others are told who
// left and drop it from their windows.
func TestWithdrawFromParty(t *testing.T) {
	g := threeMemberParty(t)
	third := g.players[2]

	third.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	frames := drainFrames(t, third.c)
	assertOpcodes(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePartySmallWindowDeleteAll}, "leaver")
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageYouLeftParty)
	for _, p := range g.players[:2] {
		frames := drainFrames(t, p.c)
		assertOpcodes(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePartySmallWindowDelete}, p.name)
		assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1LeftParty, "Third")
		if id, name := readWindowDelete(t, frames[1]); id != third.id || name != "Third" {
			t.Fatalf("%s PartySmallWindowDelete = %d %q", p.name, id, name)
		}
	}
}

// TestLastTwoDisperse pins a two-member party losing one: the party
// disperses, and both members get their window cleared and the dispersal
// message.
func TestLastTwoDisperse(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	g.invite(t, 0, 1, 0)

	g.players[1].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	for _, p := range g.players {
		frames := drainFrames(t, p.c)
		assertOpcodes(t, frames, []byte{serverpackets.OpcodePartySmallWindowDeleteAll, serverpackets.OpcodeSystemMessage}, p.name)
		assertStaticSystemMessage(t, frames[1], serverpackets.SystemMessagePartyDispersed)
	}
	// Dispersed for good: withdrawing again does nothing, and the former
	// member can be invited anew.
	g.players[1].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	assertSilent(t, g.players[1].c, "withdraw with no party")
	g.players[0].c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, g.players[0].c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Member")
}

// TestLeaderLeavingDisperses pins the leader withdrawing from a party of
// three: a leader leaving other than by disconnecting disperses the party.
func TestLeaderLeavingDisperses(t *testing.T) {
	g := threeMemberParty(t)
	g.players[0].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	for _, p := range g.players {
		frames := drainFrames(t, p.c)
		assertOpcodes(t, frames, []byte{serverpackets.OpcodePartySmallWindowDeleteAll, serverpackets.OpcodeSystemMessage}, p.name)
	}
}

// TestExpelMember pins RequestOustPartyMember: the leader expels by name,
// case-insensitively; a member who does not lead expels no one and hears
// nothing.
func TestExpelMember(t *testing.T) {
	g := threeMemberParty(t)
	leader, member, third := g.players[0], g.players[1], g.players[2]

	member.c.Send(encodeName(clientpackets.OpcodeRequestOustPartyMember, "Third"))
	assertSilent(t, member.c, "member trying to expel")
	assertSilent(t, third.c, "member a non-leader tried to expel")

	leader.c.Send(encodeName(clientpackets.OpcodeRequestOustPartyMember, "third"))
	frames := drainFrames(t, third.c)
	assertOpcodes(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePartySmallWindowDeleteAll}, "expelled")
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageHaveBeenExpelledFromParty)
	for _, p := range []player{leader, member} {
		frames := drainFrames(t, p.c)
		assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1WasExpelledFromParty, "Third")
		if id, _ := readWindowDelete(t, frames[1]); id != third.id {
			t.Fatalf("%s PartySmallWindowDelete = %d, want %d", p.name, id, third.id)
		}
	}
}

// TestChangeLeader pins RequestChangePartyLeader: only the leader hands
// over, not to itself, and the handover rebuilds every member's window
// around the new leader before telling each who leads.
func TestChangeLeader(t *testing.T) {
	g := threeMemberParty(t)
	leader, member := g.players[0], g.players[1]

	member.c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, "Member"))
	assertStaticSystemMessage(t, member.c.Read(), serverpackets.SystemMessageOnlyPartyLeaderCanTransferRights)

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, "Leader"))
	assertStaticSystemMessage(t, leader.c.Read(), serverpackets.SystemMessageYouCannotTransferRightsToYourself)

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, "Nobody"))
	assertSilent(t, leader.c, "handover to no member")

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, "Member"))
	for _, p := range g.players {
		frames := drainFrames(t, p.c)
		at := indexOf(frames, serverpackets.OpcodePartySmallWindowDeleteAll)
		if at < 0 || at+1 >= len(frames) {
			t.Fatalf("%s frames = %x, want PartySmallWindowDeleteAll then PartySmallWindowAll", p.name, opcodes(frames))
		}
		lead, _, rows := readWindowAll(t, frames[at+1])
		if lead != member.id || len(rows) != 2 {
			t.Fatalf("%s PartySmallWindowAll leader %d rows %+v, want leader %d and two rows", p.name, lead, rows, member.id)
		}
		msg := indexOfMessage(frames, serverpackets.SystemMessageS1HasBecomeAPartyLeader)
		if msg < at {
			t.Fatalf("%s frames = %x, want S1_HAS_BECOME_A_PARTY_LEADER after the rebuilt window", p.name, opcodes(frames))
		}
		assertSystemMessageText(t, frames[msg], serverpackets.SystemMessageS1HasBecomeAPartyLeader, "Member")
	}

	// The former leader no longer leads.
	leader.c.Send(encodeName(clientpackets.OpcodeRequestOustPartyMember, "Third"))
	assertSilent(t, leader.c, "former leader expelling")
}

// TestLeaderDisconnectHandsOver pins Player.deleteMe for a leader of three:
// a disconnect does not disperse the party; the next member leads, and the
// one left is dropped from the windows of those who stay.
func TestLeaderDisconnectHandsOver(t *testing.T) {
	g := threeMemberParty(t)
	leader, member, third := g.players[0], g.players[1], g.players[2]

	leader.c.Send(encodeSingle(clientpackets.OpcodeLogout))
	g.srv.AdvanceUntil(t, "leader leaves the world", func() bool {
		_, ok := g.srv.State.Player(leader.id)
		return !ok
	})
	for _, p := range []player{member, third} {
		frames := drainFrames(t, p.c)
		lead, _, rows := readWindowAll(t, frames[indexOf(frames, serverpackets.OpcodePartySmallWindowAll)])
		if lead != member.id || len(rows) != 1 {
			t.Fatalf("%s PartySmallWindowAll leader %d rows %+v, want leader %d with one other", p.name, lead, rows, member.id)
		}
		left := indexOfMessage(frames, serverpackets.SystemMessageS1LeftParty)
		if left < 0 {
			t.Fatalf("%s frames = %x, want S1_LEFT_PARTY", p.name, opcodes(frames))
		}
		assertSystemMessageText(t, frames[left], serverpackets.SystemMessageS1LeftParty, "Leader")
		if id, _ := readWindowDelete(t, frames[left+1]); id != leader.id {
			t.Fatalf("%s PartySmallWindowDelete = %d, want %d", p.name, id, leader.id)
		}
	}
	// The new leader leads: expelling from a party of two disperses it.
	member.c.Send(encodeName(clientpackets.OpcodeRequestOustPartyMember, "Third"))
	frames := drainFrames(t, third.c)
	assertStaticSystemMessage(t, frames[indexOf(frames, serverpackets.OpcodePartySmallWindowDeleteAll)+1], serverpackets.SystemMessagePartyDispersed)
}

// TestVitalsRefreshPartyWindow pins PlayerStatus.broadcastStatusUpdate's
// party branch: a member's HP change moves its row on the other members'
// windows, not its own.
func TestVitalsRefreshPartyWindow(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]
	g.invite(t, 0, 1, 0)

	// The damage itself is silent; reporting the member's status is what
	// refreshes its row.
	g.srv.DamagePlayerHP(t, member.id, 50)
	obj, ok := g.srv.State.Player(member.id)
	if !ok {
		t.Fatal("member missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	character.BroadcastStatus()
	frames := drainFrames(t, leader.c)
	at := indexOf(frames, serverpackets.OpcodePartySmallWindowUpdate)
	if at < 0 {
		t.Fatalf("leader frames = %x, want PartySmallWindowUpdate", opcodes(frames))
	}
	r := wire.NewReader(frames[at][1:])
	if id, name := r.ReadInt32(), r.ReadString(); id != member.id || name != "Member" {
		t.Fatalf("PartySmallWindowUpdate member = %d %q", id, name)
	}
	r.ReadInt32()
	r.ReadInt32()
	if hp, maxHP := r.ReadInt32(), r.ReadInt32(); hp >= maxHP || maxHP != int32(g.srv.PlayerMaxHP(t, member.id)) {
		t.Fatalf("PartySmallWindowUpdate HP = %d/%d, want below the member's max %d", hp, maxHP, g.srv.PlayerMaxHP(t, member.id))
	}
	if indexOf(drainFrames(t, member.c), serverpackets.OpcodePartySmallWindowUpdate) >= 0 {
		t.Fatal("member got its own PartySmallWindowUpdate")
	}
}

func indexOf(frames [][]byte, opcode byte) int {
	for i, f := range frames {
		if f[0] == opcode {
			return i
		}
	}
	return -1
}

func indexOfMessage(frames [][]byte, id int) int {
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == int32(id) {
			return i
		}
	}
	return -1
}
