package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// declare has the founder declare war on name and returns both sides'
// answers.
func (w *clanWorld) declare(t *testing.T, name string) (leader, rival [][]byte) {
	t.Helper()
	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, name))
	return drainFrames(t, w.leader), drainFrames(t, w.member)
}

// wantMessages fails unless the system messages among frames are want.
func wantMessages(t *testing.T, what string, frames [][]byte, want ...int) {
	t.Helper()
	if got := messages(t, frames); !slices.Equal(got, want) {
		t.Fatalf("%s messages = %v, want %v", what, got, want)
	}
}

// TestClanWarDeclareAndStop walks a war from its refusals to its end: a
// clan below level 3, then a target below level 3, an unknown clan and the
// clan itself are refused; the declaration reaches both clans with their
// header, notice and status refresh, and is stored; the war lists show it
// from both sides; the target clan can neither stop nor surrender a war it
// did not declare; stopping it tells both clans and stores the five-day
// penalty, which refuses a new declaration.
func TestClanWarDeclareAndStop(t *testing.T) {
	w := bootWarWorld(t)
	knightsID := w.found(t, "Knights")
	if _, ok := firstOpcode(w.masterCommandBy(t, w.member, "create_clan Rivals"), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("rival founded no clan")
	}
	drainFrames(t, w.leader)
	rivalsID := int32(queryInt(t, w, `SELECT clan_id FROM clan_data WHERE clan_name = 'Rivals'`))

	leader, _ := w.declare(t, "Rivals")
	wantMessages(t, "level 0 declaration", leader, serverpackets.SystemMessageWarNeedsLevel3Or15Members)
	w.raiseToLevel3(t, w.leader)
	leader, _ = w.declare(t, "Rivals")
	if len(leader) != 1 {
		t.Fatalf("weak target refusal = %x", opcodes(leader))
	}
	if id, params := sysMsg(t, leader[0]); id != serverpackets.SystemMessageS1ClanCannotDeclareWarTooWeak || !slices.Equal(params, []string{"Rivals"}) {
		t.Fatalf("weak target refusal = %d %v", id, params)
	}
	w.raiseToLevel3(t, w.member)
	leader, _ = w.declare(t, "Nobody")
	wantMessages(t, "unknown clan", leader, serverpackets.SystemMessageWarClanDoesNotExist)
	leader, _ = w.declare(t, "knights")
	wantMessages(t, "own clan", leader, serverpackets.SystemMessageCannotDeclareAgainstOwnClan)

	leader, rival := w.declare(t, "rivals")
	keep := []byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo}
	want := string(keep)
	if got := string(only(leader, keep...)); got != want {
		t.Fatalf("declarer's answer = %x, want %x", got, want)
	}
	if got := string(only(rival, keep...)); got != want {
		t.Fatalf("target's notice = %x, want %x", got, want)
	}
	header, _ := firstOpcode(leader, serverpackets.OpcodePledgeShowInfoUpdate)
	if headerAtWar(t, header) != 1 {
		t.Fatal("declaring clan's header not at war")
	}
	header, _ = firstOpcode(rival, serverpackets.OpcodePledgeShowInfoUpdate)
	if headerAtWar(t, header) != 0 {
		t.Fatal("target's header at war before it declares")
	}
	notice, _ := firstOpcode(leader, serverpackets.OpcodeSystemMessage)
	if id, params := sysMsg(t, notice); id != serverpackets.SystemMessageWarDeclaredAgainstS1 || !slices.Equal(params, []string{"Rivals"}) {
		t.Fatalf("declarer's notice = %d %v", id, params)
	}
	notice, _ = firstOpcode(rival, serverpackets.OpcodeSystemMessage)
	if id, params := sysMsg(t, notice); id != serverpackets.SystemMessageClanS1DeclaredWar || !slices.Equal(params, []string{"Knights"}) {
		t.Fatalf("target's notice = %d %v", id, params)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE clan1 = ? AND clan2 = ? AND expiry_time = 0`, knightsID, rivalsID); got != 1 {
		t.Fatalf("stored wars = %d, want 1", got)
	}

	leader, _ = w.declare(t, "Rivals")
	wantMessages(t, "second declaration", leader, serverpackets.SystemMessageWarAlreadyDeclared)

	w.leader.Send(encodeRequestPledgeWarList(0, 0))
	lists := extended(drainFrames(t, w.leader), serverpackets.OpcodeExPledgeReceiveWarList)
	if len(lists) != 1 {
		t.Fatal("no war list")
	}
	if tab, page, count, names := warList(t, lists[0]); tab != 0 || page != 0 || count != 1 || !slices.Equal(names, []string{"Rivals"}) {
		t.Fatalf("declared tab = %d %d %d %v", tab, page, count, names)
	}
	w.member.Send(encodeRequestPledgeWarList(3, 1))
	lists = extended(drainFrames(t, w.member), serverpackets.OpcodeExPledgeReceiveWarList)
	if len(lists) != 1 {
		t.Fatal("no attacker list")
	}
	if tab, page, count, names := warList(t, lists[0]); tab != 1 || page != 0 || count != 1 || !slices.Equal(names, []string{"Knights"}) {
		t.Fatalf("attacker tab = %d %d %d %v, want the page reset to 0", tab, page, count, names)
	}

	w.member.Send(encodeWarName(clientpackets.OpcodeRequestStopPledgeWar, "Knights"))
	wantMessages(t, "target's stop", drainFrames(t, w.member), serverpackets.SystemMessageNotInvolvedInWar)
	w.member.Send(encodeWarName(clientpackets.OpcodeRequestSurrenderPledgeWar, "Knights"))
	wantMessages(t, "target's surrender", drainFrames(t, w.member), serverpackets.SystemMessageNotInvolvedInWar)
	for _, opcode := range []byte{clientpackets.OpcodeRequestReplyStartPledgeWar, clientpackets.OpcodeRequestReplyStopPledgeWar, clientpackets.OpcodeRequestReplySurrenderPledgeWar} {
		w.member.Send(encodeWarReply(opcode, 1))
		if frames := drainFrames(t, w.member); len(frames) != 0 {
			t.Fatalf("war reply %#x = %x, want nothing", opcode, opcodes(frames))
		}
	}

	w.srv.SetPlayerInCombat(t, w.leaderID, true)
	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStopPledgeWar, "Rivals"))
	wantMessages(t, "stop in combat", drainFrames(t, w.leader), serverpackets.SystemMessageCannotStopWarInCombat)
	w.srv.SetPlayerInCombat(t, w.leaderID, false)

	before := time.Now().UnixMilli()
	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStopPledgeWar, "Rivals"))
	leader, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
	if got := string(only(leader, keep...)); got != want {
		t.Fatalf("stopper's answer = %x, want %x", got, want)
	}
	header, _ = firstOpcode(leader, serverpackets.OpcodePledgeShowInfoUpdate)
	if headerAtWar(t, header) != 0 {
		t.Fatal("stopping clan's header still at war")
	}
	wantMessages(t, "stopper", leader, serverpackets.SystemMessageWarAgainstS1Stopped)
	wantMessages(t, "target of the stop", rival, serverpackets.SystemMessageClanS1DecidedToStopWar)
	w.srv.FlushPersistence(t)
	expiry := queryInt(t, w, `SELECT expiry_time FROM clan_wars WHERE clan1 = ? AND clan2 = ?`, knightsID, rivalsID)
	if day := int64(24 * time.Hour / time.Millisecond); expiry < before+5*day || expiry > time.Now().UnixMilli()+5*day {
		t.Fatalf("stored penalty = %d, want five days from the stop", expiry)
	}

	leader, _ = w.declare(t, "Rivals")
	if len(leader) != 1 {
		t.Fatalf("declaration under penalty = %x", opcodes(leader))
	}
	if id, params := sysMsg(t, leader[0]); id != serverpackets.SystemMessageAlreadyAtWarWithS1Wait5Days || !slices.Equal(params, []string{"Rivals"}) {
		t.Fatalf("declaration under penalty = %d %v", id, params)
	}
}

// TestClanWarSurrender has the rival leader, whose clan declared war,
// surrender: it loses a full death's experience, is told under the name it
// typed, and both clans learn the war ended.
func TestClanWarSurrender(t *testing.T) {
	w := bootWarWorld(t)
	w.found(t, "Knights")
	w.masterCommandBy(t, w.member, "create_clan Rivals")
	drainFrames(t, w.leader)
	w.raiseToLevel3(t, w.leader)
	w.raiseToLevel3(t, w.member)
	w.member.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, "Knights"))
	drainFrames(t, w.member)
	drainFrames(t, w.leader)

	w.member.Send(encodeWarName(clientpackets.OpcodeRequestSurrenderPledgeWar, "knights"))
	rival, leader := drainFrames(t, w.member), drainFrames(t, w.leader)
	wantMessages(t, "surrendering leader", rival,
		serverpackets.SystemMessageYouHaveSurrenderedToS1Clan, serverpackets.SystemMessageWarAgainstS1Stopped)
	surrender, _ := firstOpcode(rival, serverpackets.OpcodeSystemMessage)
	if _, params := sysMsg(t, surrender); !slices.Equal(params, []string{"knights"}) {
		t.Fatalf("surrender notice names %v, want the typed name", params)
	}
	if order := only(rival, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate); len(order) < 3 || order[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("surrender answer = %x, want the experience loss's UserInfo first", order)
	}
	wantMessages(t, "surrendered-to clan", leader, serverpackets.SystemMessageClanS1DecidedToStopWar)

	w.leaveWorld(t, w.member)
	// Level 10 spans 48229-71201: 8.875% of 22972 is 2039.
	if got := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.memberID); got != 60000-2039 {
		t.Fatalf("exp after surrender = %d, want %d", got, 60000-2039)
	}
}
