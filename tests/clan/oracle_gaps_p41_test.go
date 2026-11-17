package clan

import (
	"database/sql"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Evidence for the clan refusals the first parity sweep left without a
// scenario: a clan creation under its penalty (VillageMaster.java:160-213,
// ClanTable.createClan), a withdrawal or an expulsion of a connected member
// in combat (RequestWithdrawalPledge, RequestOustPledgeMember.java:53), an
// invitation into a full main clan (Clan.checkClanJoinCondition,
// Clan.java:1746-1754) and an academy sponsor request without master rights
// (RequestPledgeSetAcademyMaster.java:36-40).
//
// Not covered here, each owned by an open issue that rewrites the
// assertion: the title on join and on a noble's leave, and the timing of a
// re-requested dissolution.

const fullClanID int32 = 0x7f000002

// seedFullMainClan seeds the founder's level-0 clan "Knights" with nine
// offline members, so its main roster holds the 10 a level-0 clan allows.
func seedFullMainClan(t *testing.T) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		stmts := []string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				SELECT ` + itoa(fullClanID) + `, 'Knights', 0, obj_Id FROM characters WHERE char_name = 'Founder'`,
			`UPDATE characters SET clanid = ` + itoa(fullClanID) + `, power_grade = 0 WHERE char_name = 'Founder'`,
		}
		for i := range 9 {
			id, name := itoa(squireID+int32(i)), "Squire"+strconv.Itoa(i)
			stmts = append(stmts, `INSERT INTO characters (account_name, obj_Id, char_name, level, classid, clanid, power_grade, sex, race)
				VALUES ('`+name+`', `+id+`, '`+name+`', 20, 0, `+itoa(fullClanID)+`, 6, 0, 0)`)
		}
		seedStatements(t, db, stmts...)
	})
}

// TestClanCreationRefusedUnderPenalty has a founder whose creation penalty
// has not run out ask the village master for a clan: it is answered
// YOU_MUST_WAIT_XX_DAYS_BEFORE_CREATING_A_NEW_CLAN and no clan exists.
func TestClanCreationRefusedUnderPenalty(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanSeed(func(db *sql.DB) {
		seedStatements(t, db, `UPDATE characters SET clan_create_expiry_time = 99999999999999 WHERE char_name = 'Founder'`)
	}))
	w.talkToMaster(t)

	frames := w.masterCommand(t, "create_clan Knights")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("create_clan under penalty answered %x, want SystemMessage, ActionFailed", got)
	}
	if id, _ := sysMsg(t, frames[0]); id != serverpackets.SystemMessageMustWaitBeforeCreatingClan {
		t.Fatalf("create_clan under penalty message = %d, want %d", id, serverpackets.SystemMessageMustWaitBeforeCreatingClan)
	}
	if _, ok := w.srv.Clans.Table().ByName("Knights"); ok {
		t.Fatal("Knights was founded under the creation penalty")
	}
}

// TestWithdrawRefusedInCombat has a member in combat ask to leave its
// clan: YOU_CANNOT_LEAVE_DURING_COMBAT answers it, and it stays a member.
func TestWithdrawRefusedInCombat(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)
	w.srv.SetPlayerInCombat(t, w.memberID, true)

	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	frames := drainFrames(t, w.member)
	if got := messages(t, frames); !slices.Equal(got, []int{serverpackets.SystemMessageCannotLeaveDuringCombat}) || len(frames) != 1 {
		t.Fatalf("withdrawal in combat answered %x (messages %v), want only YOU_CANNOT_LEAVE_DURING_COMBAT", opcodes(frames), got)
	}
	if cl, _ := w.srv.Clans.Table().Get(clanID); !cl.IsMember(w.memberID) {
		t.Fatal("the member in combat left its clan")
	}
}

// TestOustRefusedForConnectedMemberInCombat has the leader expel a
// connected member in combat: CLAN_MEMBER_CANNOT_BE_DISMISSED_DURING_COMBAT
// answers it, and the member stays on the roster, untold.
func TestOustRefusedForConnectedMemberInCombat(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)
	w.srv.SetPlayerInCombat(t, w.memberID, true)

	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	frames := drainFrames(t, w.leader)
	if got := messages(t, frames); !slices.Equal(got, []int{serverpackets.SystemMessageClanMemberCannotBeDismissedCombat}) || len(frames) != 1 {
		t.Fatalf("expulsion in combat answered %x (messages %v), want only CLAN_MEMBER_CANNOT_BE_DISMISSED_DURING_COMBAT", opcodes(frames), got)
	}
	if got := drainFrames(t, w.member); len(got) != 0 {
		t.Fatalf("member in combat got %x, want nothing", opcodes(got))
	}
	if cl, _ := w.srv.Clans.Table().Get(clanID); !cl.IsMember(w.memberID) {
		t.Fatal("the member in combat was expelled")
	}
}

// TestInvitationRefusedByFullMainClan has the leader of a level-0 clan
// holding its 10 members invite a recruit: S1_CLAN_IS_FULL names the clan,
// and the recruit is sent nothing.
func TestInvitationRefusedByFullMainClan(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0, seedFullMainClan(t))

	id, params := w.inviteRefusal(t)
	if id != serverpackets.SystemMessageS1ClanIsFull || !slices.Equal(params, []string{"Knights"}) {
		t.Fatalf("invitation into a full clan = %d %v, want S1_CLAN_IS_FULL Knights", id, params)
	}
}

// TestAcademyMasterRequestRefusedWithoutMasterRights has a plain member
// ask to link a sponsor and an apprentice: it is told it has no right to
// dismiss an apprentice, and nothing is linked.
func TestAcademyMasterRequestRefusedWithoutMasterRights(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.member.Send(encodeRequestPledgeSetAcademyMaster(1, "Recruit", "Founder"))
	frames := drainFrames(t, w.member)
	if got := messages(t, frames); !slices.Equal(got, []int{serverpackets.SystemMessageNoRightToDismissApprentice}) || len(frames) != 1 {
		t.Fatalf("sponsor request without rights answered %x (messages %v), want only NO_RIGHT_TO_DISMISS_APPRENTICE", opcodes(frames), got)
	}
	if got := drainFrames(t, w.leader); len(got) != 0 {
		t.Fatalf("leader got %x, want nothing", opcodes(got))
	}
}
