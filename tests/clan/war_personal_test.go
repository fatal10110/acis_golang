package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Seeded personal-surrender cast: a Knights member and a clanless player.
const (
	sentryID  int32 = 0x7f200201
	drifterID int32 = 0x7f200202
)

// bootSurrenderWorld boots Knights and Rivals with stmts seeding their
// wars, plus the Knights member Sentry (account player4) and the clanless
// Drifter (account player5), both left out of the world.
func bootSurrenderWorld(t *testing.T, stmts ...string) *clanWorld {
	t.Helper()
	stmts = append([]string{
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(sentryID),
	}, stmts...)
	return bootAllianceCast(t, []castMember{{sentryID, "player4", "Sentry"}, {drifterID, "player5", "Drifter"}}, stmts)
}

// surrenderPersonally sends c's personal surrender to the clan named name
// and returns c's answer.
func surrenderPersonally(t *testing.T, c *testsupport.ScriptedClient, name string) [][]byte {
	t.Helper()
	c.Send(encodeWarName(clientpackets.OpcodeRequestSurrenderPersonally, name))
	return drainFrames(t, c)
}

// TestPersonalSurrenderRefusals has the request refused with
// FAILED_TO_PERSONALLY_SURRENDER when the player's clan did not declare war
// on the named clan, and go unanswered from a clanless player or for a
// name no clan bears.
func TestPersonalSurrenderRefusals(t *testing.T) {
	t.Parallel()
	w := bootSurrenderWorld(t, warStmt(rivalsClanID, knightsClanID))

	if frames := surrenderPersonally(t, w.leader, "Nobody"); len(frames) != 0 {
		t.Fatalf("surrender to no clan = %x, want nothing", opcodes(frames))
	}
	wantMessages(t, "surrender to a clan not at war", surrenderPersonally(t, w.leader, "Rivals"),
		serverpackets.SystemMessageFailedToPersonallySurrender)
	drifter := w.srv.DialClient(t, "player5", 1)
	startInWorld(t, drifter)
	if frames := surrenderPersonally(t, drifter, "Knights"); len(frames) != 0 {
		t.Fatalf("clanless surrender = %x, want nothing", opcodes(frames))
	}
	w.leaveWorld(t, w.leader)
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, w.leaderID); got != 0 {
		t.Fatalf("refused surrender stored wantspeace = %d, want 0", got)
	}
}

// TestPersonalSurrenderEndsWar has the founder of Knights, at mutual war
// with Rivals, surrender personally while Sentry, its other member, is
// online: it loses a full death's experience, is told under the name it
// typed, and, every member but one now wanting peace, both wars end: each
// clan is shown its header out of war and its stop notice, Knights keeps
// the five-day penalty and Rivals' war on it is gone. The flag is stored.
func TestPersonalSurrenderEndsWar(t *testing.T) {
	t.Parallel()
	w := bootSurrenderWorld(t, warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID))
	sentry := w.srv.DialClient(t, "player4", 1)
	startInWorld(t, sentry)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	before := time.Now().UnixMilli()
	founder := surrenderPersonally(t, w.leader, "rivals")
	rival, member := drainFrames(t, w.member), drainFrames(t, sentry)
	wantMessages(t, "surrendering founder", founder,
		serverpackets.SystemMessageYouHavePersonallySurrenderedToS1Clan, serverpackets.SystemMessageWarAgainstS1Stopped)
	notice, _ := firstOpcode(founder, serverpackets.OpcodeSystemMessage)
	if _, params := sysMsg(t, notice); !slices.Equal(params, []string{"rivals"}) {
		t.Fatalf("surrender notice names %v, want the typed name", params)
	}
	want := []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage}
	if got := only(founder, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate); !slices.Equal(got, want) {
		t.Fatalf("founder's answer = %x, want the experience loss's UserInfo, the notice, then the war's end %x", got, want)
	}
	wantMessages(t, "other Knights member", member, serverpackets.SystemMessageWarAgainstS1Stopped)
	wantMessages(t, "Rivals", rival, serverpackets.SystemMessageClanS1DecidedToStopWar)
	for who, frames := range map[string][][]byte{"founder": founder, "member": member, "rival": rival} {
		header, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate)
		if !ok {
			t.Fatalf("%s got no header", who)
		}
		if headerAtWar(t, header) != 0 {
			t.Fatalf("%s's header still at war", who)
		}
	}

	w.leaveWorld(t, w.leader)
	expiry := queryInt(t, w, `SELECT expiry_time FROM clan_wars WHERE clan1 = ? AND clan2 = ?`, knightsClanID, rivalsClanID)
	if day := int64(24 * time.Hour / time.Millisecond); expiry < before+5*day || expiry > time.Now().UnixMilli()+5*day {
		t.Fatalf("stored Knights penalty = %d, want five days from the surrender", expiry)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE clan1 = ? AND clan2 = ?`, rivalsClanID, knightsClanID); got != 0 {
		t.Fatalf("stored Rivals wars on Knights = %d, want 0", got)
	}
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, w.leaderID); got != 1 {
		t.Fatalf("stored wantspeace = %d, want 1", got)
	}
	if got := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.leaderID); got != foundExp-fullDeathLoss {
		t.Fatalf("exp after the surrender = %d, want %d", got, foundExp-fullDeathLoss)
	}
}

// TestPersonalSurrenderWaitsForOfflineMember has the founder surrender
// personally while Sentry, who surrendered before, is offline: the war goes
// on, as an offline member cannot be counted, and a second surrender is
// refused. Sentry's stored flag comes back with it, refusing its own
// surrender, and leaving the clan clears the flag for good. The founder's
// flag is stored on logout.
func TestPersonalSurrenderWaitsForOfflineMember(t *testing.T) {
	t.Parallel()
	w := bootSurrenderWorld(t, warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID),
		`UPDATE characters SET wantspeace = 1 WHERE obj_Id = `+itoa(sentryID))

	founder := surrenderPersonally(t, w.leader, "Rivals")
	wantMessages(t, "surrender with a member offline", founder, serverpackets.SystemMessageYouHavePersonallySurrenderedToS1Clan)
	if _, ok := firstOpcode(founder, serverpackets.OpcodePledgeShowInfoUpdate); ok {
		t.Fatal("war ended with a member offline")
	}
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("Rivals told %x of a war still on", opcodes(frames))
	}
	wantMessages(t, "second surrender", surrenderPersonally(t, w.leader, "Rivals"), serverpackets.SystemMessageFailedToPersonallySurrender)

	sentry := w.srv.DialClient(t, "player4", 1)
	startInWorld(t, sentry)
	drainFrames(t, w.leader)
	wantMessages(t, "restored surrender", surrenderPersonally(t, sentry, "Rivals"), serverpackets.SystemMessageFailedToPersonallySurrender)

	sentry.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	drainFrames(t, sentry)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, sentryID); got != 0 {
		t.Fatalf("wantspeace after leaving = %d, want 0", got)
	}
	w.leaveWorld(t, sentry)
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, sentryID); got != 0 {
		t.Fatalf("wantspeace saved after leaving = %d, want 0", got)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE expiry_time = 0`); got != 2 {
		t.Fatalf("stored wars = %d, want both still on", got)
	}
	w.leaveWorld(t, w.leader)
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, w.leaderID); got != 1 {
		t.Fatalf("founder's stored wantspeace = %d, want 1", got)
	}
}
