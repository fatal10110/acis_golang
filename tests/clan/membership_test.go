package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestInviteAndJoin walks an invitation: the recruit is asked by name, and
// on accepting joins as a rank 6 member. The recruit gets JoinPledge, its
// row, its status, ENTERED_THE_CLAN, the clan header and the roster, then
// its refreshed status; the leader learns who joined and gets the new row
// and the header.
func TestInviteAndJoin(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	asked := drainFrames(t, w.member)
	if got := opcodes(asked); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAskJoinPledge}) {
		t.Fatalf("invitation = %x, want SystemMessage, AskJoinPledge", got)
	}
	if id, params := sysMsg(t, asked[0]); id != serverpackets.SystemMessageS1HasInvitedYouToJoinTheClanS2 || !slices.Equal(params, []string{"Founder", "Knights"}) {
		t.Fatalf("invitation message = %d %v, want S1_HAS_INVITED_YOU_TO_JOIN_THE_CLAN_S2 Founder Knights", id, params)
	}
	r := wire.NewReader(asked[1][1:])
	if requester, name := r.ReadInt32(), r.ReadString(); requester != w.leaderID || name != "Knights" {
		t.Fatalf("AskJoinPledge = %d %q, want %d Knights", requester, name, w.leaderID)
	}

	w.member.Send(encodeRequestAnswerJoinPledge(1))
	joined := drainFrames(t, w.member)
	want := []byte{
		serverpackets.OpcodeJoinPledge, serverpackets.OpcodePledgeShowMemberListUpdate, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodePledgeShowMemberListAll,
		serverpackets.OpcodeUserInfo,
	}
	if got := opcodes(joined); string(got) != string(want) {
		t.Fatalf("recruit's join = %x, want %x", got, want)
	}
	if id := wire.NewReader(joined[0][1:]).ReadInt32(); id != clanID {
		t.Fatalf("JoinPledge clan = %d, want %d", id, clanID)
	}
	if id, _ := sysMsg(t, joined[3]); id != serverpackets.SystemMessageEnteredTheClan {
		t.Fatalf("recruit's message = %d, want ENTERED_THE_CLAN", id)
	}

	leader := drainFrames(t, w.leader)
	want = []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListAdd, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeCharInfo}
	if got := opcodes(leader); string(got) != string(want) {
		t.Fatalf("leader's view of the join = %x, want %x", got, want)
	}
	if id, params := sysMsg(t, leader[0]); id != serverpackets.SystemMessageS1HasJoinedClan || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("leader's message = %d %v, want S1_HAS_JOINED_CLAN Recruit", id, params)
	}
	r = wire.NewReader(leader[1][1:])
	if name := r.ReadString(); name != "Recruit" {
		t.Fatalf("PledgeShowMemberListAdd name = %q, want Recruit", name)
	}
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if online := r.ReadInt32(); online != w.memberID {
		t.Fatalf("new row online id = %d, want %d", online, w.memberID)
	}

	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT clanid FROM characters WHERE obj_Id = ? AND power_grade = 6 AND subpledge = 0 AND title = ''`, w.memberID); got != int64(clanID) {
		t.Fatalf("recruit's clanid = %d, want %d", got, clanID)
	}

	// Invitations a member could not accept are refused to the inviter.
	for _, tc := range []struct {
		name   string
		target int32
		want   int
		params []string
	}{
		{"already a member", w.memberID, serverpackets.SystemMessageS1WorkingWithAnotherClan, []string{"Recruit"}},
		{"oneself", w.leaderID, serverpackets.SystemMessageCannotInviteYourself, nil},
		{"nobody", 999999, serverpackets.SystemMessageYouHaveInvitedTheWrongTarget, nil},
	} {
		w.leader.Send(encodeRequestJoinPledge(tc.target, 0))
		frames := drainFrames(t, w.leader)
		if len(frames) != 1 {
			t.Fatalf("%s: answer = %x, want one SystemMessage", tc.name, opcodes(frames))
		}
		if id, params := sysMsg(t, frames[0]); id != tc.want || !slices.Equal(params, tc.params) {
			t.Fatalf("%s: message = %d %v, want %d %v", tc.name, id, params, tc.want, tc.params)
		}
	}
	// A rank 6 member holds no invitation privilege.
	w.member.Send(encodeRequestJoinPledge(w.leaderID, 0))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageNotAuthorizedToDoThat}) {
		t.Fatalf("member's invitation = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
	}
}

// TestInvitationRefusedAndBusy refuses an invitation: each side is told,
// and nothing changes. A clanless player's invitation is ignored, and a
// requester whose invitation is pending is told to wait.
func TestInvitationRefusedAndBusy(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)

	// Clanless: ignored.
	w.member.Send(encodeRequestJoinPledge(w.leaderID, 0))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("clanless invitation answer = %x, want nothing", opcodes(frames))
	}

	w.found(t, "Knights")
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	drainFrames(t, w.member)
	// A second invitation while the first is pending.
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	frames := drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("second invitation answer = %x, want one SystemMessage", opcodes(frames))
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1IsBusyTryLater || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("second invitation message = %d %v, want S1_IS_BUSY_TRY_LATER Recruit", id, params)
	}

	w.member.Send(encodeRequestAnswerJoinPledge(0))
	frames = drainFrames(t, w.member)
	if len(frames) != 1 {
		t.Fatalf("refusal answer = %x, want one SystemMessage", opcodes(frames))
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageYouDidNotRespondToS1ClanInvitation || !slices.Equal(params, []string{"Founder"}) {
		t.Fatalf("refuser's message = %d %v, want YOU_DID_NOT_RESPOND_TO_S1_CLAN_INVITATION Founder", id, params)
	}
	frames = drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("inviter's answer = %x, want one SystemMessage", opcodes(frames))
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1DidNotRespondToClanInvitation || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("inviter's message = %d %v, want S1_DID_NOT_RESPOND_TO_CLAN_INVITATION Recruit", id, params)
	}
	// The inviter stays busy until its own side lapses.
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageWaitingForAnotherReply}) {
		t.Fatalf("invitation right after a refusal = %v, want WAITING_FOR_ANOTHER_REPLY", ids)
	}
	// An answer with nothing pending is ignored.
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("answer without invitation = %x, want nothing", opcodes(frames))
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COALESCE(clanid,0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("refuser's clanid = %d, want 0", got)
	}
}

// TestWithdraw has the recruit leave: it gets its skills, status and
// cleared clan window, then the two leave messages; the leader learns it
// left and loses its row. The leaver may not join again for the join
// penalty, the leader may not withdraw, and a clanless player is told it
// has no clan.
func TestWithdraw(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageClanLeaderCannotWithdraw}) {
		t.Fatalf("leader's withdrawal = %v, want CLAN_LEADER_CANNOT_WITHDRAW", ids)
	}

	before := time.Now().UnixMilli()
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	left := drainFrames(t, w.member)
	want := []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage}
	if got := opcodes(left); string(got) != string(want) {
		t.Fatalf("leaver's answer = %x, want %x", got, want)
	}
	if ids := messages(t, left); !slices.Equal(ids, []int{serverpackets.SystemMessageYouHaveWithdrawnFromClan, serverpackets.SystemMessageMustWaitBeforeJoiningAnotherClan}) {
		t.Fatalf("leaver's messages = %v", ids)
	}
	leader := drainFrames(t, w.leader)
	if got := only(leader, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListDelete); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListDelete}) {
		t.Fatalf("leader's view = %x", opcodes(leader))
	}
	frame, _ := firstOpcode(leader, serverpackets.OpcodeSystemMessage)
	if id, params := sysMsg(t, frame); id != serverpackets.SystemMessageS1HasWithdrawnFromTheClan || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("leader's message = %d %v, want S1_HAS_WITHDRAWN_FROM_THE_CLAN Recruit", id, params)
	}

	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ? AND clanid = 0 AND title = ''`, w.memberID); got < before+24*3600*1000 {
		t.Fatalf("leaver's join penalty ends %d, want a day after %d", got, before)
	}

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	frames := drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("re-invitation answer = %x", opcodes(frames))
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1MustWaitBeforeJoiningAnotherClan || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("re-invitation message = %d %v, want S1_MUST_WAIT_BEFORE_JOINING_ANOTHER_CLAN Recruit", id, params)
	}

	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageYouAreNotAClanMember}) {
		t.Fatalf("clanless withdrawal = %v, want YOU_ARE_NOT_A_CLAN_MEMBER", ids)
	}
}

// TestOust expels the recruit: it learns its membership ended after its
// clan window closes; the clan loses its row and is told; the leader may
// not recruit for the penalty. A member without the dismiss privilege, an
// expulsion of oneself and of an unknown name are refused or ignored.
func TestOust(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.member.Send(encodeRequestOustPledgeMember("Founder"))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageNotAuthorizedToDoThat}) {
		t.Fatalf("member's expulsion = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
	}
	w.leader.Send(encodeRequestOustPledgeMember("Founder"))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageCannotDismissYourself}) {
		t.Fatalf("self expulsion = %v, want YOU_CANNOT_DISMISS_YOURSELF", ids)
	}
	w.leader.Send(encodeRequestOustPledgeMember("Nobody"))
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("unknown expulsion = %x, want nothing", opcodes(frames))
	}

	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	leader := drainFrames(t, w.leader)
	if got := only(leader, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListDelete); string(got) != string([]byte{
		serverpackets.OpcodePledgeShowMemberListDelete, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
	}) {
		t.Fatalf("leader's expulsion answer = %x", opcodes(leader))
	}
	if ids := messages(t, leader); !slices.Equal(ids, []int{
		serverpackets.SystemMessageClanMemberS1Expelled, serverpackets.SystemMessageSucceededInExpellingClanMember, serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember,
	}) {
		t.Fatalf("leader's messages = %v", ids)
	}
	ousted := drainFrames(t, w.member)
	want := []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodeSystemMessage}
	if got := opcodes(ousted); string(got) != string(want) {
		t.Fatalf("ousted answer = %x, want %x", got, want)
	}
	if id, _ := sysMsg(t, ousted[3]); id != serverpackets.SystemMessageClanMembershipTerminated {
		t.Fatalf("ousted message = %d, want CLAN_MEMBERSHIP_TERMINATED", id)
	}

	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_data WHERE char_penalty_expiry_time > ?`, time.Now().UnixMilli()); got != 1 {
		t.Fatalf("clans under recruiting penalty = %d, want 1", got)
	}
	if got := queryInt(t, w, `SELECT COALESCE(clanid,0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("ousted clanid = %d, want 0", got)
	}
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember}) {
		t.Fatalf("invitation under the recruiting penalty = %v, want YOU_MUST_WAIT_BEFORE_ACCEPTING_A_NEW_MEMBER", ids)
	}
}

// TestOustOfflineMember expels a member that is not in the world: its row
// is cleared in place.
func TestOustOfflineMember(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)
	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)

	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{
		serverpackets.SystemMessageClanMemberS1Expelled, serverpackets.SystemMessageSucceededInExpellingClanMember, serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember,
	}) {
		t.Fatalf("offline expulsion messages = %v", ids)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ? AND clanid = 0 AND title = ''`, w.memberID); got <= time.Now().UnixMilli() {
		t.Fatalf("offline ousted join penalty = %d, want in the future", got)
	}
}
