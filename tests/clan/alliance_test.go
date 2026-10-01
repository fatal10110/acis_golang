package clan

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const dayMs = int64(24 * time.Hour / time.Millisecond)

// TestAllianceFormsAndReports founds an alliance at the village master,
// invites the rival clan's leader, who accepts, then shows the alliance
// window and carries alliance chat to both clans.
func TestAllianceFormsAndReports(t *testing.T) {
	w := bootAllianceWorld(t)

	frames := w.masterCommandBy(t, w.leader, "create_ally Ally")
	if got := only(frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage); !bytes.Equal(got, []byte{serverpackets.OpcodeUserInfo}) {
		t.Fatalf("create_ally answer = %x, want UserInfo alone", opcodes(frames))
	}
	if got := storedAlly(t, w, knightsClanID); got.allyID != int64(knightsClanID) || got.allyName != "Ally" || got.penaltyType != 0 {
		t.Fatalf("founded alliance row = %+v", got)
	}
	drainFrames(t, w.member)

	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	asked := drainFrames(t, w.member)
	if got := opcodes(asked); !bytes.Equal(got, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAskJoinAlly}) {
		t.Fatalf("invitation = %x, want SystemMessage, AskJoinAlly", got)
	}
	if id, params := sysMsg(t, asked[0]); id != allyMsgRequested || !slices.Equal(params, []string{"Ally", "Founder"}) {
		t.Fatalf("invitation message = %d %v, want %d [Ally Founder]", id, params, allyMsgRequested)
	}
	r := wire.NewReader(asked[1][1:])
	if requester, name := r.ReadInt32(), r.ReadString(); requester != w.leaderID || name != "Ally" {
		t.Fatalf("AskJoinAlly = %d %q, want %d \"Ally\"", requester, name, w.leaderID)
	}
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("inviter's answer = %x, want nothing", opcodes(frames))
	}

	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 1))
	accepted := drainFrames(t, w.member)
	if got := only(accepted, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage); !bytes.Equal(got, []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("acceptance = %x, want UserInfo then YOU_ACCEPTED_ALLIANCE", opcodes(accepted))
	}
	if ids := messages(t, accepted); !slices.Equal(ids, []int{allyMsgAccepted}) {
		t.Fatalf("acceptance messages = %v", ids)
	}
	if got := storedAlly(t, w, rivalsClanID); got.allyID != int64(knightsClanID) || got.allyName != "Ally" {
		t.Fatalf("joined clan row = %+v", got)
	}

	w.member.Send(encodeAllyBare(clientpackets.OpcodeRequestAllyInfo))
	info := drainFrames(t, w.member)
	if info[0][0] != serverpackets.OpcodeAllianceInfo {
		t.Fatalf("alliance info = %x, want AllianceInfo first", opcodes(info))
	}
	r = wire.NewReader(info[0][1:])
	if name, total, online, leaderClan, leaderName, clans := r.ReadString(), r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString(), r.ReadInt32(); name != "Ally" || total != 2 || online != 2 || leaderClan != "Knights" || leaderName != "Founder" || clans != 2 {
		t.Fatalf("AllianceInfo header = %q %d %d %q %q %d", name, total, online, leaderClan, leaderName, clans)
	}
	for _, want := range []string{"Knights", "Rivals"} {
		if got := r.ReadString(); got != want {
			t.Fatalf("AllianceInfo clan = %q, want %q", got, want)
		}
		r.ReadInt32()
		r.ReadInt32()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
	}
	wantMsgs := []int{
		allyMsgInfoHead, allyMsgInfoName, allyMsgInfoLeader, allyMsgConnection, allyMsgClanTotal,
		allyMsgClanHead, allyMsgClanName, allyMsgClanLeader, allyMsgClanLevel, allyMsgConnection,
		allyMsgClanSeparator, allyMsgClanName, allyMsgClanLeader, allyMsgClanLevel, allyMsgConnection,
		allyMsgClanFoot,
	}
	if ids := messages(t, info); !slices.Equal(ids, wantMsgs) {
		t.Fatalf("alliance info messages = %v, want %v", ids, wantMsgs)
	}
	if _, params := sysMsg(t, info[3]); !slices.Equal(params, []string{"Knights", "Founder"}) {
		t.Fatalf("alliance leader line = %v", params)
	}
	if _, params := sysMsg(t, info[4]); !slices.Equal(params, []string{"2", "2"}) {
		t.Fatalf("alliance connection line = %v, want online 2 of 2", params)
	}

	w.member.Send(encodeSay(sayAlliance, "hello allies"))
	for _, c := range []struct {
		name   string
		frames [][]byte
	}{{"rival", drainFrames(t, w.member)}, {"founder", drainFrames(t, w.leader)}} {
		say, ok := firstOpcode(c.frames, serverpackets.OpcodeCreatureSay)
		if !ok {
			t.Fatalf("%s heard no alliance line: %x", c.name, opcodes(c.frames))
		}
		r := wire.NewReader(say[1:])
		if obj, typ, speaker, text := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString(); obj != w.memberID || typ != sayAlliance || speaker != "Rival" || text != "hello allies" {
			t.Fatalf("%s heard %d %d %q %q", c.name, obj, typ, speaker, text)
		}
	}
}

// TestAllianceRefusals walks the alliance requests' refusals, each
// answered by its own system message.
func TestAllianceRefusals(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.MembersForWar = 1
	w := bootAllianceWorld(t, gameservertest.WithClanConfig(cfg))

	expect := func(t *testing.T, name string, frames [][]byte, want int) {
		t.Helper()
		if ids := messages(t, frames); !slices.Equal(ids, []int{want}) {
			t.Fatalf("%s messages = %v, want [%d]", name, ids, want)
		}
	}
	send := func(c []byte) [][]byte {
		w.leader.Send(c)
		return drainFrames(t, w.leader)
	}

	expect(t, "invite with no alliance", send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID)), allyMsgFeatureOnlyForLeader)
	expect(t, "alliance info with no alliance", send(encodeAllyBare(clientpackets.OpcodeRequestAllyInfo)), allyMsgNoCurrentAlliances)
	expect(t, "leave with no alliance", send(encodeAllyBare(clientpackets.OpcodeAllyLeave)), allyMsgNoCurrentAlliances)
	expect(t, "dissolve with no alliance", send(encodeAllyBare(clientpackets.OpcodeRequestDismissAlly)), allyMsgNoCurrentAlliances)
	expect(t, "create_ally with a bad name", w.masterCommandBy(t, w.leader, "create_ally Bad_Name"), allyMsgIncorrectName)
	expect(t, "create_ally with a short name", w.masterCommandBy(t, w.leader, "create_ally A"), allyMsgIncorrectNameLength)

	w.masterCommandBy(t, w.leader, "create_ally Ally")
	drainFrames(t, w.member)
	expect(t, "create_ally twice", w.masterCommandBy(t, w.leader, "create_ally Other"), allyMsgAlreadyJoined)
	expect(t, "create_ally with a taken name", w.masterCommandBy(t, w.member, "create_ally ally"), allyMsgAlreadyExists)
	expect(t, "invite oneself", send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.leaderID)), allyMsgCannotInviteYourself)
	expect(t, "invite nobody", send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, 999999)), serverpackets.SystemMessageYouHaveInvitedTheWrongTarget)
	expect(t, "alliance leader leaves", send(encodeAllyBare(clientpackets.OpcodeAllyLeave)), allyMsgLeaderCantWithdraw)
	expect(t, "dismiss one's own clan", send(encodeAllyName(clientpackets.OpcodeAllyDismiss, "Knights")), allyMsgLeaderCantWithdraw)
	expect(t, "dismiss a clan that does not exist", send(encodeAllyName(clientpackets.OpcodeAllyDismiss, "Ghosts")), allyMsgClanDoesntExist)
	expect(t, "dismiss a clan of no alliance", send(encodeAllyName(clientpackets.OpcodeAllyDismiss, "Rivals")), allyMsgDifferentAlliance)

	w.member.Send(encodeAllyBare(clientpackets.OpcodeAllyLeave))
	expect(t, "leave from a clan of no alliance", drainFrames(t, w.member), allyMsgNoCurrentAlliances)
	w.member.Send(encodeAllyBare(clientpackets.OpcodeRequestDismissAlly))
	expect(t, "dissolve from a clan of no alliance", drainFrames(t, w.member), allyMsgNoCurrentAlliances)
	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.leaderID))
	expect(t, "invite from outside an alliance", drainFrames(t, w.member), allyMsgFeatureOnlyForLeader)

	// A war the leading clan declared on the invited clan refuses it.
	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, "Rivals"))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	expect(t, "invite a clan at war", send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID)), allyMsgMayNotAllyClanBattle)
}

// TestAllianceLimit refuses an invitation into a full alliance.
func TestAllianceLimit(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.MaxClansInAlly = 1
	w := bootAllianceWorld(t, gameservertest.WithClanConfig(cfg))
	w.masterCommandBy(t, w.leader, "create_ally Ally")
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{allyMsgExceededTheLimit}) {
		t.Fatalf("invitation into a full alliance = %v, want [%d]", ids, allyMsgExceededTheLimit)
	}
}

// TestAllianceInvitationDeclined answers a declined invitation on both
// sides and leaves the invited clan out of the alliance.
func TestAllianceInvitationDeclined(t *testing.T) {
	w := bootAllianceWorld(t)
	w.masterCommandBy(t, w.leader, "create_ally Ally")
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	drainFrames(t, w.member)
	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 0))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{allyMsgYouDidNotRespond}) {
		t.Fatalf("decliner's messages = %v", ids)
	}
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{allyMsgNoResponse}) {
		t.Fatalf("inviter's messages after a decline = %v", ids)
	}
	if got := storedAlly(t, w, rivalsClanID); got.allyID != 0 {
		t.Fatalf("declining clan's row = %+v, want no alliance", got)
	}
}

// TestAllianceLeavePenalty has the rival clan leave the alliance, checking
// its stored penalty and the refusal it causes.
func TestAllianceLeavePenalty(t *testing.T) {
	w := bootAllianceWorld(t)
	w.formAlliance(t)

	before := time.Now().UnixMilli()
	w.member.Send(encodeAllyBare(clientpackets.OpcodeAllyLeave))
	left := drainFrames(t, w.member)
	if ids := messages(t, left); !slices.Equal(ids, []int{allyMsgWithdrawn}) {
		t.Fatalf("leave messages = %v", ids)
	}
	if _, ok := firstOpcode(left, serverpackets.OpcodeUserInfo); !ok {
		t.Fatalf("leave answer = %x, want the rival's UserInfo refreshed", opcodes(left))
	}
	got := storedAlly(t, w, rivalsClanID)
	if got.allyID != 0 || got.allyName != "" || got.penaltyType != clan.AllyPenaltyClanLeft ||
		got.penaltyExpiry < before+dayMs || got.penaltyExpiry > time.Now().UnixMilli()+dayMs {
		t.Fatalf("left clan row = %+v, want a one-day leave penalty", got)
	}
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	invite := drainFrames(t, w.leader)
	if ids := messages(t, invite); !slices.Equal(ids, []int{allyMsgCantEnterWithin1DayS1}) {
		t.Fatalf("invitation of a clan that left = %v", ids)
	}
	refusal, _ := firstOpcode(invite, serverpackets.OpcodeSystemMessage)
	if _, params := sysMsg(t, refusal); !slices.Equal(params, []string{"Rivals"}) {
		t.Fatalf("invitation refusal names %v, want [Rivals]", params)
	}
}

// TestAllianceDismissPenalties has the alliance leader dismiss the rival
// clan, naming it in any case, checking both sides' stored penalties and
// the refusal the leader's causes.
func TestAllianceDismissPenalties(t *testing.T) {
	w := bootAllianceWorld(t)
	w.formAlliance(t)
	before := time.Now().UnixMilli()
	w.leader.Send(encodeAllyName(clientpackets.OpcodeAllyDismiss, "rivals"))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{allyMsgExpelledAClan}) {
		t.Fatalf("dismissal messages = %v", ids)
	}
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("dismissed clan's leader was not refreshed")
	}
	dismissed, leader := storedAlly(t, w, rivalsClanID), storedAlly(t, w, knightsClanID)
	if dismissed.allyID != 0 || dismissed.penaltyType != clan.AllyPenaltyClanDismissed || dismissed.penaltyExpiry < before+dayMs {
		t.Fatalf("dismissed clan row = %+v", dismissed)
	}
	if leader.allyID != int64(knightsClanID) || leader.penaltyType != clan.AllyPenaltyDismissClan || leader.penaltyExpiry < before+dayMs {
		t.Fatalf("dismissing clan row = %+v", leader)
	}
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{allyMsgCantInviteWithin1Day}) {
		t.Fatalf("invitation after a dismissal = %v", ids)
	}
}

// TestAllianceCrestAndDissolution has the alliance leader upload an
// alliance crest, which every clan of the alliance shows, then dissolve
// the alliance: both clans learn it, lose the alliance and its crest, the
// image is deleted, the leading clan may not found another for ten days and
// its leader loses a death's experience.
func TestAllianceCrestAndDissolution(t *testing.T) {
	dir := t.TempDir()
	w := bootAllianceWorld(t, gameservertest.WithCrests(datacache.NewCrestsIn(dir)))
	w.formAlliance(t)

	image := bytes.Repeat([]byte{0x5a}, clientpackets.AllyCrestMaxLength)
	w.member.Send(encodeSetAllyCrest(image))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("crest upload by a member clan = %x, want nothing", opcodes(frames))
	}
	w.leader.Send(encodeSetAllyCrest(image))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{msgEmblemRegistered}) {
		t.Fatalf("crest upload messages = %v", ids)
	}
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("member clan's leader was not refreshed with the new crest")
	}
	crestID := storedAlly(t, w, knightsClanID).allyCrestID
	if crestID == 0 || storedAlly(t, w, rivalsClanID).allyCrestID != crestID {
		t.Fatalf("alliance crest columns = %d / %d, want the same new id", crestID, storedAlly(t, w, rivalsClanID).allyCrestID)
	}
	path := filepath.Join(dir, "AllyCrest_"+strconv.FormatInt(crestID, 10)+".dds")
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, image) {
		t.Fatalf("stored crest image: %v", err)
	}

	before := time.Now().UnixMilli()
	w.leader.Send(encodeAllyBare(clientpackets.OpcodeRequestDismissAlly))
	dissolved := drainFrames(t, w.leader)
	if ids := messages(t, dissolved); len(ids) == 0 || ids[0] != allyMsgDissolved {
		t.Fatalf("dissolver's messages = %v, want ALLIANCE_DISSOLVED first", ids)
	}
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{allyMsgDissolved}) {
		t.Fatalf("member clan's messages = %v", ids)
	}
	leader, member := storedAlly(t, w, knightsClanID), storedAlly(t, w, rivalsClanID)
	if leader.allyID != 0 || leader.allyCrestID != 0 || leader.penaltyType != clan.AllyPenaltyDissolveAlly || leader.penaltyExpiry < before+10*dayMs {
		t.Fatalf("dissolving clan row = %+v", leader)
	}
	if member.allyID != 0 || member.allyName != "" || member.allyCrestID != 0 || member.penaltyType != 0 {
		t.Fatalf("former member clan row = %+v", member)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("crest image after dissolution: %v, want deleted", err)
	}
	// Level 10 spans 22972 experience; 8.875% of it is 2039.
	w.leaveWorld(t, w.leader)
	if exp := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.leaderID); exp != 60000-2039 {
		t.Fatalf("dissolver's experience = %d, want %d", exp, 60000-2039)
	}
}

// TestAllianceDissolvePenaltyRefusesFounding refuses a new alliance to a
// clan that dissolved one, and to a clan below level 5.
func TestAllianceDissolvePenaltyRefusesFounding(t *testing.T) {
	future := strconv.FormatInt(time.Now().UnixMilli()+dayMs, 10)
	w := bootAllianceWorldSeeded(t, []string{
		`UPDATE clan_data SET ally_penalty_expiry_time = ` + future + `, ally_penalty_type = 4 WHERE clan_id = ` + itoa(knightsClanID),
		`UPDATE clan_data SET clan_level = 4 WHERE clan_id = ` + itoa(rivalsClanID),
	})
	if ids := messages(t, w.masterCommandBy(t, w.leader, "create_ally Ally")); !slices.Equal(ids, []int{allyMsgCantCreate10DaysDissol}) {
		t.Fatalf("founding under a dissolution penalty = %v", ids)
	}
	if ids := messages(t, w.masterCommandBy(t, w.member, "create_ally Ally")); !slices.Equal(ids, []int{allyMsgLevel5Needed}) {
		t.Fatalf("founding by a level-4 clan = %v", ids)
	}
}

// TestAllianceDanglingDroppedAtBoot clears, at boot, the alliance of a
// clan whose alliance's leading clan no longer exists.
func TestAllianceDanglingDroppedAtBoot(t *testing.T) {
	w := bootAllianceWorldSeeded(t, []string{
		`UPDATE clan_data SET ally_id = 2130706687, ally_name = 'Ghost', ally_crest_id = 5 WHERE clan_id = ` + itoa(rivalsClanID),
	})
	if got := storedAlly(t, w, rivalsClanID); got.allyID != 0 || got.allyName != "" || got.allyCrestID != 0 {
		t.Fatalf("dangling alliance row after boot = %+v, want cleared", got)
	}
}
