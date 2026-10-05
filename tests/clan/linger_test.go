package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A clan member whose connection dropped in combat lingers in the world 15
// s before it leaves (GameClient.onDisconnection, GameClient.java:201-213),
// its client already detached. ClanMember.isOnline is false for it
// (ClanMember.java:174-177), so the roster, the online counts and
// Clan.getOnlineMembers treat it as offline while it lingers.

// dropInCombat closes c, objID's connection, while it is in combat, and
// returns once the server noticed the drop, the player still in the world.
func (w *clanWorld) dropInCombat(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	w.srv.SetPlayerInCombat(t, objID, true)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	obj, ok := w.srv.State.Player(objID)
	if !ok {
		t.Fatal("the dropped member left the world at once")
	}
	for deadline := time.Now().Add(5 * time.Second); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			t.Fatal("the server never noticed the connection drop")
		}
		time.Sleep(time.Millisecond)
	}
}

// stillLingers fails unless objID is still in the world.
func (w *clanWorld) stillLingers(t *testing.T, objID int32) {
	t.Helper()
	if _, ok := w.srv.State.Player(objID); !ok {
		t.Fatal("the dropped member left the world before the check")
	}
}

// rosterOnline decodes a PledgeShowMemberListAll frame (PledgeShowMemberListAll.java)
// into each member's online field by name.
func rosterOnline(t *testing.T, frame []byte) map[string]int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowMemberListAll {
		t.Fatalf("opcode = %#x, want PledgeShowMemberListAll", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // sub-unit
	r.ReadInt32() // clan id
	r.ReadInt32() // pledge type
	r.ReadString()
	r.ReadString()
	for range 7 { // crest, level, castle, hall, rank, reputation, dissolving
		r.ReadInt32()
	}
	r.ReadInt32() // 0
	r.ReadInt32() // ally id
	r.ReadString()
	r.ReadInt32() // ally crest
	r.ReadInt32() // at war
	out := map[string]int32{}
	for n := r.ReadInt32(); n > 0; n-- {
		name := r.ReadString()
		for range 4 { // level, class, sex, race
			r.ReadInt32()
		}
		out[name] = r.ReadInt32()
		r.ReadInt32() // sponsor
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode PledgeShowMemberListAll: %v", err)
	}
	return out
}

// TestLingeringMemberListedOffline asks for the roster while the recruit
// lingers: its row reads offline, the leader's online.
func TestLingeringMemberListedOffline(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.dropInCombat(t, w.member, w.memberID)
	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	frame, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatal("roster request answered no PledgeShowMemberListAll")
	}
	online := rosterOnline(t, frame)
	if got := online["Recruit"]; got != 0 {
		t.Fatalf("lingering recruit's online field = %d, want 0", got)
	}
	if got := online["Founder"]; got != w.leaderID {
		t.Fatalf("leader's online field = %d, want %d", got, w.leaderID)
	}
	w.stillLingers(t, w.memberID)
}

// TestLingeringMemberNotCountedInAllianceInfo shows the alliance window
// while the rival clan's leader lingers: one member of two online
// (ClanInfo.java:13, Clan.getOnlineMembersCount).
func TestLingeringMemberNotCountedInAllianceInfo(t *testing.T) {
	w := bootAllianceWorld(t)
	w.formAlliance(t)

	w.dropInCombat(t, w.member, w.memberID)
	w.leader.Send(encodeAllyBare(clientpackets.OpcodeRequestAllyInfo))
	info := drainFrames(t, w.leader)
	if len(info) == 0 || info[0][0] != serverpackets.OpcodeAllianceInfo {
		t.Fatalf("alliance info = %x, want AllianceInfo first", opcodes(info))
	}
	r := wire.NewReader(info[0][1:])
	r.ReadString()
	if total, online := r.ReadInt32(), r.ReadInt32(); total != 2 || online != 1 {
		t.Fatalf("AllianceInfo total %d online %d, want 2 and 1", total, online)
	}
	if _, params := sysMsg(t, info[4]); len(params) != 2 || params[0] != "1" || params[1] != "2" {
		t.Fatalf("alliance connection line = %v, want online 1 of 2", params)
	}
	w.stillLingers(t, w.memberID)
}

// TestLingeringMemberDoesNotHoldWarStop stops a war while a member of the
// stopping clan lingers in combat: the reference only checks the online
// members' combat (RequestStopPledgeWar.java:46-53), so the war ends.
func TestLingeringMemberDoesNotHoldWarStop(t *testing.T) {
	w := bootWarWorld(t)
	w.found(t, "Knights")
	if _, ok := firstOpcode(w.masterCommandBy(t, w.member, "create_clan Rivals"), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("rival founded no clan")
	}
	drainFrames(t, w.leader)
	w.raiseToLevel3(t, w.leader)
	w.raiseToLevel3(t, w.member)

	squire, squireID := w.enterExtra(t, "player3", "Squire")
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.leader.Send(encodeRequestJoinPledge(squireID, 0))
	drainFrames(t, squire)
	squire.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, squire), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("squire did not join")
	}
	drainFrames(t, w.leader)
	if declared, _ := w.declare(t, "Rivals"); !slices.Contains(messages(t, declared), serverpackets.SystemMessageWarDeclaredAgainstS1) {
		t.Fatalf("declaration answer = %x, want the war declared", opcodes(declared))
	}

	w.dropInCombat(t, squire, squireID)
	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStopPledgeWar, "Rivals"))
	stop := drainFrames(t, w.leader)
	for _, id := range messages(t, stop) {
		if id == serverpackets.SystemMessageCannotStopWarInCombat {
			t.Fatal("a lingering member's combat refused stopping the war")
		}
	}
	if header, ok := firstOpcode(stop, serverpackets.OpcodePledgeShowInfoUpdate); !ok || headerAtWar(t, header) != 0 {
		t.Fatalf("stop answer = %x, want the header no longer at war", opcodes(stop))
	}
	w.stillLingers(t, squireID)
}
