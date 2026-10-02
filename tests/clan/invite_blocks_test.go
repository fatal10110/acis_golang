package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: Clan.checkClanJoinCondition (Clan.java:1686-1760) refuses,
// right after an invitation of oneself, a target blocking everything with
// S1_BLOCKED_EVERYTHING and a target whose block list holds the inviter
// with S1_HAS_ADDED_YOU_TO_IGNORE_LIST, each naming the target; the
// acceptance (RequestAnswerJoinPledge.java:47) runs the same check with the
// accepting player as the target. aCis revision in the outer repo.

func encodeBlock(typ int32, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(typ)
	if typ == clientpackets.BlockAdd || typ == clientpackets.BlockRemove {
		w.WriteString(name)
	}
	return w.Bytes()
}

// inviteRefusal has the founder invite the recruit and returns the one
// system message the founder is answered with; the recruit is sent
// nothing.
func (w *clanWorld) inviteRefusal(t *testing.T) (int, []string) {
	t.Helper()
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	frames := drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("invitation answer = %x, want one SystemMessage", opcodes(frames))
	}
	if got := drainFrames(t, w.member); len(got) != 0 {
		t.Fatalf("refused target got %x, want nothing", opcodes(got))
	}
	return sysMsg(t, frames[0])
}

// TestInvitationRefusedByTargetsBlocks refuses an invitation of a recruit
// blocking everything, then of one whose block list holds the founder,
// each naming the recruit; once both blocks are lifted the invitation
// reaches it.
func TestInvitationRefusedByTargetsBlocks(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")

	w.member.Send(encodeBlock(clientpackets.BlockAll, ""))
	w.member.Send(encodeBlock(clientpackets.BlockAdd, "Founder"))
	drainFrames(t, w.member)
	drainFrames(t, w.leader)
	if id, params := w.inviteRefusal(t); id != serverpackets.SystemMessageS1BlockedEverything || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("invitation of a recruit blocking everything = %d %v, want S1_BLOCKED_EVERYTHING Recruit", id, params)
	}

	w.member.Send(encodeBlock(clientpackets.BlockAllRelease, ""))
	drainFrames(t, w.member)
	if id, params := w.inviteRefusal(t); id != serverpackets.SystemMessageS1HasAddedYouToIgnoreList || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("invitation of a recruit blocking the founder = %d %v, want S1_HAS_ADDED_YOU_TO_IGNORE_LIST Recruit", id, params)
	}

	w.member.Send(encodeBlock(clientpackets.BlockRemove, "Founder"))
	drainFrames(t, w.member)
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("invitation after the blocks were lifted did not reach the recruit")
	}
}

// TestAcceptanceRefusedByAcceptersBlocks has the recruit, invited, start
// blocking everything, or block the founder, before it accepts: the
// acceptance is refused to the founder naming the recruit, the recruit is
// told nothing and stays clanless.
func TestAcceptanceRefusedByAcceptersBlocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		block  []byte
		wantID int
	}{
		{"blocking everything", encodeBlock(clientpackets.BlockAll, ""), serverpackets.SystemMessageS1BlockedEverything},
		{"blocking the founder", encodeBlock(clientpackets.BlockAdd, "Founder"), serverpackets.SystemMessageS1HasAddedYouToIgnoreList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootClanWorld(t, 10, 0, 0)
			w.found(t, "Knights")
			w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
			if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
				t.Fatal("invitation did not reach the recruit")
			}
			w.member.Send(tc.block)
			drainFrames(t, w.member)
			drainFrames(t, w.leader)

			w.member.Send(encodeRequestAnswerJoinPledge(1))
			if frames := drainFrames(t, w.member); len(frames) != 0 {
				t.Fatalf("refused accepter got %x, want nothing", opcodes(frames))
			}
			frames := drainFrames(t, w.leader)
			if len(frames) != 1 {
				t.Fatalf("inviter's answer = %x, want one SystemMessage", opcodes(frames))
			}
			if id, params := sysMsg(t, frames[0]); id != tc.wantID || !slices.Equal(params, []string{"Recruit"}) {
				t.Fatalf("inviter's message = %d %v, want %d Recruit", id, params, tc.wantID)
			}
			w.srv.FlushPersistence(t)
			if got := queryInt(t, w, `SELECT COALESCE(clanid,0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
				t.Fatalf("refused accepter's clanid = %d, want 0", got)
			}
		})
	}
}
