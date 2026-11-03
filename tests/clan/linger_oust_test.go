package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: RequestOustPledgeMember refuses an expulsion for combat only
// when the member is online (RequestOustPledgeMember.java:53), and a member
// lingering after its connection dropped is not online
// (ClanMember.isOnline), so its combat holds nothing back. Its client is
// gone, so it is told nothing.
//
// Decided divergence (#3667): the reference then takes the offline removal
// path, leaving the lingering character its clan, rank and academy join
// level in memory, which its store at the end of the linger writes back
// over the removal, so the character returns to the clan's roster after a
// restart and keeps its academy join level clanless. That corruption is not
// reproduced: the lingering character leaves the clan as an online one
// does, and its row stays clanless once it leaves.

// TestOustLingeringAcademyMember drops the academy recruit in combat and
// has the founder expel it while it lingers: the expulsion is not refused
// for combat, the clan sees it expelled, and once the recruit has left the
// world its row reads clanless, with no rank, sub-unit or academy join
// level.
func TestOustLingeringAcademyMember(t *testing.T) {
	t.Parallel()
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t, academySeed))
	w.leader.Send(encodeRequestJoinPledge(w.memberID, -1))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	drainFrames(t, w.member)
	drainFrames(t, w.leader)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT lvl_joined_academy FROM characters WHERE obj_Id = ?`, w.memberID); got != 1 {
		t.Fatalf("stored academy join level = %d, want 1", got)
	}

	w.srv.DropInCombat(t, w.memberID, w.member)
	drainFrames(t, w.leader)
	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	answer := drainFrames(t, w.leader)
	if got := messages(t, answer); !slices.Equal(got, []int{
		serverpackets.SystemMessageClanMemberS1Expelled,
		serverpackets.SystemMessageSucceededInExpellingClanMember,
		serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember,
	}) {
		t.Fatalf("expulsion of the lingering recruit answered %v, want it expelled", got)
	}
	if _, ok := firstOpcode(answer, serverpackets.OpcodePledgeShowMemberListDelete); !ok {
		t.Fatalf("expulsion answer = %x, want the recruit's row deleted", opcodes(answer))
	}
	w.stillLingers(t, w.memberID)
	if cl, _ := w.srv.Clans.Table().Get(subunitClanID); cl.IsMember(w.memberID) {
		t.Fatal("the lingering recruit is still listed in the clan")
	}

	// The in-combat linger is 15 s, past AdvanceUntil's limit.
	for start := w.leader.Now(); ; w.srv.Advance(t, 100*time.Millisecond) {
		if _, ok := w.srv.State.Player(w.memberID); !ok {
			break
		}
		if w.leader.Now().Sub(start) > 30*time.Second {
			t.Fatal("the lingering recruit was still in the world 30 s later")
		}
	}
	w.srv.FlushPersistence(t)
	for column, want := range map[string]int64{"clanid": 0, "power_grade": 0, "subpledge": 0, "lvl_joined_academy": 0} {
		if got := queryInt(t, w, `SELECT `+column+` FROM characters WHERE obj_Id = ?`, w.memberID); got != want {
			t.Errorf("stored %s after the linger = %d, want %d", column, got, want)
		}
	}
}
