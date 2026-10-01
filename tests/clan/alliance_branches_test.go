package clan

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The alliance tests' extra cast: Squire, a member of Rivals who does not
// lead it; Loner, in no clan; Third, leading Thirds.
const (
	allySquireID int32 = 0x7f200002
	lonerID      int32 = 0x7f200003
	thirdID      int32 = 0x7f200004
	thirdsClanID int32 = 0x7f000013
)

// The alliance refusals naming the invited player's side.
const (
	allyMsgTargetMustBeInClan  = 234
	allyMsgCantEnterWithin1Day = 468
)

var (
	allySquire = castMember{allySquireID, "player3", "Squire"}
	loner      = castMember{lonerID, "player4", "Loner"}
	third      = castMember{thirdID, "player5", "Third"}
)

// squireInRivals makes Squire an ordinary member of Rivals.
func squireInRivals() string {
	return `UPDATE characters SET clanid = ` + itoa(rivalsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(allySquireID)
}

// thirdsClan seeds Thirds, a level 5 clan led by Third, with the alliance
// columns given.
func thirdsClan(allyColumns string) []string {
	return []string{
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, ally_id, ally_name, ally_crest_id)
			VALUES (` + itoa(thirdsClanID) + `, 'Thirds', 5, ` + itoa(thirdID) + `, ` + allyColumns + `)`,
		`UPDATE characters SET clanid = ` + itoa(thirdsClanID) + `, power_grade = 0 WHERE obj_Id = ` + itoa(thirdID),
	}
}

// bringIn brings m into the world beside the founder and the rival.
func (w *clanWorld) bringIn(t *testing.T, m castMember) *testsupport.ScriptedClient {
	t.Helper()
	c := w.srv.DialClient(t, m.account, 1)
	startInWorld(t, c)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return c
}

// expectMessages fails unless frames carry exactly the system messages
// want.
func expectMessages(t *testing.T, name string, frames [][]byte, want ...int) {
	t.Helper()
	if ids := messages(t, frames); !slices.Equal(ids, want) {
		t.Fatalf("%s messages = %v, want %v", name, ids, want)
	}
}

// expectParams fails unless the first system message among frames names
// want.
func expectParams(t *testing.T, name string, frames [][]byte, want ...string) {
	t.Helper()
	msg, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage)
	if !ok {
		t.Fatalf("%s: no system message in %x", name, opcodes(frames))
	}
	if _, params := sysMsg(t, msg); !slices.Equal(params, want) {
		t.Fatalf("%s names %v, want %v", name, params, want)
	}
}

// TestAllianceInvitationTargetRefusals refuses an invitation from a
// clanless player, then the alliance leader's invitations of a member who
// does not lead its clan, of a clanless player, of the leader of a clan
// already in an alliance and of a clan dismissed from an alliance a day
// ago, each answered to the inviter alone; an ordinary member may not
// withdraw its clan from an alliance.
func TestAllianceInvitationTargetRefusals(t *testing.T) {
	future := strconv.FormatInt(time.Now().UnixMilli()+dayMs, 10)
	stmts := append([]string{
		squireInRivals(),
		`UPDATE clan_data SET ally_penalty_expiry_time = ` + future + `, ally_penalty_type = 2 WHERE clan_id = ` + itoa(rivalsClanID),
	}, thirdsClan(itoa(thirdsClanID)+`, 'Outer', 0`)...)
	w := bootAllianceCast(t, []castMember{allySquire, loner, third}, stmts)
	sq, lo, th := w.bringIn(t, allySquire), w.bringIn(t, loner), w.bringIn(t, third)

	lo.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.leaderID))
	expectMessages(t, "invitation from a clanless player", drainFrames(t, lo), allyMsgNotAClanMember)

	w.masterCommandBy(t, w.leader, "create_ally Ally")
	for _, c := range []*testsupport.ScriptedClient{sq, lo, th, w.member} {
		drainFrames(t, c)
	}
	invite := func(target int32) [][]byte {
		w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, target))
		return drainFrames(t, w.leader)
	}
	frames := invite(allySquireID)
	expectMessages(t, "invitation of a member who does not lead its clan", frames, allyMsgIsNotAClanLeader)
	expectParams(t, "invitation of a member who does not lead its clan", frames, "Squire")
	expectMessages(t, "invitation of a clanless player", invite(lonerID), allyMsgTargetMustBeInClan)
	frames = invite(thirdID)
	expectMessages(t, "invitation of an allied clan", frames, allyMsgAlreadyMemberOfAlly)
	expectParams(t, "invitation of an allied clan", frames, "Thirds", "Outer")
	expectMessages(t, "invitation of a dismissed clan", invite(rivalLeaderID), allyMsgCantEnterWithin1Day)
	for name, c := range map[string]*testsupport.ScriptedClient{"Squire": sq, "Loner": lo, "Third": th, "Rival": w.member} {
		if frames := drainFrames(t, c); len(frames) != 0 {
			t.Fatalf("refused invitation reached %s: %x", name, opcodes(frames))
		}
	}

	sq.Send(encodeAllyBare(clientpackets.OpcodeAllyLeave))
	expectMessages(t, "withdrawal by an ordinary member", drainFrames(t, sq), allyMsgOnlyLeaderWithdraw)
	if got := storedAlly(t, w, rivalsClanID); got.allyID != 0 || got.penaltyType != clan.AllyPenaltyClanDismissed {
		t.Fatalf("refused clan's row = %+v, want no alliance and its dismissal penalty", got)
	}
}

// TestAllianceMemberClanRefusals has the member clan's leader, then an
// ordinary member of that clan, try to dismiss a clan from, dissolve or
// leave the alliance: only the alliance leader may dismiss or dissolve,
// and only a clan leader may withdraw. Nothing changes.
func TestAllianceMemberClanRefusals(t *testing.T) {
	w := bootAllianceCast(t, []castMember{allySquire}, []string{squireInRivals()})
	w.formAlliance(t)
	sq := w.bringIn(t, allySquire)

	send := func(c *testsupport.ScriptedClient, packet []byte) [][]byte {
		c.Send(packet)
		return drainFrames(t, c)
	}
	expectMessages(t, "dismissal by the member clan's leader",
		send(w.member, encodeAllyName(clientpackets.OpcodeAllyDismiss, "Knights")), allyMsgFeatureOnlyForLeader)
	expectMessages(t, "dissolution by the member clan's leader",
		send(w.member, encodeAllyBare(clientpackets.OpcodeRequestDismissAlly)), allyMsgFeatureOnlyForLeader)
	expectMessages(t, "dismissal by an ordinary member",
		send(sq, encodeAllyName(clientpackets.OpcodeAllyDismiss, "Rivals")), allyMsgFeatureOnlyForLeader)
	expectMessages(t, "dissolution by an ordinary member",
		send(sq, encodeAllyBare(clientpackets.OpcodeRequestDismissAlly)), allyMsgFeatureOnlyForLeader)
	expectMessages(t, "withdrawal by an ordinary member",
		send(sq, encodeAllyBare(clientpackets.OpcodeAllyLeave)), allyMsgOnlyLeaderWithdraw)
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("refused requests reached the alliance leader: %x", opcodes(frames))
	}

	for _, id := range []int32{knightsClanID, rivalsClanID} {
		if got := storedAlly(t, w, id); got.allyID != int64(knightsClanID) || got.penaltyType != 0 {
			t.Fatalf("clan %d row after refused requests = %+v, want still in Ally without penalty", id, got)
		}
	}
}

// TestAllianceAcceptanceRechecked has the invited clan found its own
// alliance before accepting: the acceptance is refused to the inviter by
// the invitation rules, adds nothing, and leaves the invitation pending,
// so a later refusal still answers both sides.
func TestAllianceAcceptanceRechecked(t *testing.T) {
	w := bootAllianceWorld(t)
	w.masterCommandBy(t, w.leader, "create_ally Ally")
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	drainFrames(t, w.leader)
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinAlly); !ok {
		t.Fatal("rival was not asked to join")
	}
	if ids := messages(t, w.masterCommandBy(t, w.member, "create_ally Rivalry")); len(ids) != 0 {
		t.Fatalf("rival's founding = %v, want founded", ids)
	}

	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 1))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("refused acceptance answered the rival: %x", opcodes(frames))
	}
	refusal := drainFrames(t, w.leader)
	expectMessages(t, "inviter after a refused acceptance", refusal, allyMsgAlreadyMemberOfAlly)
	expectParams(t, "inviter after a refused acceptance", refusal, "Rivals", "Rivalry")
	if got := storedAlly(t, w, rivalsClanID); got.allyID != int64(rivalsClanID) || got.allyName != "Rivalry" {
		t.Fatalf("rival's row after a refused acceptance = %+v, want its own alliance", got)
	}
	if got := len(w.srv.Clans.Table().Allies(knightsClanID)); got != 1 {
		t.Fatalf("Ally holds %d clans after a refused acceptance, want 1", got)
	}

	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 0))
	expectMessages(t, "rival declining the still-pending invitation", drainFrames(t, w.member), allyMsgYouDidNotRespond)
	expectMessages(t, "inviter after the decline", drainFrames(t, w.leader), allyMsgNoResponse)
}

// TestAllianceAcceptanceIgnoresClanInvitation has a clanless player
// accept an alliance invitation while a clan invitation is pending: it is
// ignored, and the clan invitation can still be accepted.
func TestAllianceAcceptanceIgnoresClanInvitation(t *testing.T) {
	w := bootAllianceCast(t, []castMember{loner}, nil)
	lo := w.bringIn(t, loner)
	w.leader.Send(encodeRequestJoinPledge(lonerID, 0))
	drainFrames(t, w.leader)
	if _, ok := firstOpcode(drainFrames(t, lo), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("loner was not asked to join Knights")
	}

	lo.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 1))
	if frames := drainFrames(t, lo); len(frames) != 0 {
		t.Fatalf("alliance acceptance of a clan invitation answered the loner: %x", opcodes(frames))
	}
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("alliance acceptance of a clan invitation reached the inviter: %x", opcodes(frames))
	}

	lo.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, lo), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("the clan invitation was not left pending")
	}
}

// TestAllianceCrestReplacedAndDeleted has the alliance leader replace the
// alliance crest, which removes the first image, then delete it: every
// clan of the alliance loses it, the image is removed and the leader is
// told; deleting a crest that is not set is ignored.
func TestAllianceCrestReplacedAndDeleted(t *testing.T) {
	dir := t.TempDir()
	w := bootAllianceWorld(t, gameservertest.WithCrests(datacache.NewCrestsIn(dir)))
	w.formAlliance(t)
	crestPath := func(id int64) string {
		return filepath.Join(dir, "AllyCrest_"+strconv.FormatInt(id, 10)+".dds")
	}

	w.leader.Send(encodeSetAllyCrest(bytes.Repeat([]byte{0x11}, clientpackets.AllyCrestMaxLength)))
	expectMessages(t, "first crest upload", drainFrames(t, w.leader), msgEmblemRegistered)
	drainFrames(t, w.member)
	first := storedAlly(t, w, knightsClanID).allyCrestID

	replacement := bytes.Repeat([]byte{0x22}, clientpackets.AllyCrestMaxLength)
	w.leader.Send(encodeSetAllyCrest(replacement))
	expectMessages(t, "crest replacement", drainFrames(t, w.leader), msgEmblemRegistered)
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("member clan's leader was not refreshed with the replacement crest")
	}
	second := storedAlly(t, w, knightsClanID).allyCrestID
	if second == 0 || second == first || storedAlly(t, w, rivalsClanID).allyCrestID != second {
		t.Fatalf("crest columns after replacing %d = %d / %d, want both on a new id",
			first, second, storedAlly(t, w, rivalsClanID).allyCrestID)
	}
	if _, err := os.Stat(crestPath(first)); !os.IsNotExist(err) {
		t.Fatalf("replaced crest image: %v, want removed", err)
	}
	if data, err := os.ReadFile(crestPath(second)); err != nil || !bytes.Equal(data, replacement) {
		t.Fatalf("replacement crest image: %v", err)
	}

	w.leader.Send(encodeSetAllyCrest(nil))
	expectMessages(t, "crest deletion", drainFrames(t, w.leader), msgCrestDeleted)
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("member clan's leader was not refreshed without the crest")
	}
	for _, id := range []int32{knightsClanID, rivalsClanID} {
		if got := storedAlly(t, w, id); got.allyCrestID != 0 || got.allyID != int64(knightsClanID) {
			t.Fatalf("clan %d row after the crest deletion = %+v, want no crest, still in Ally", id, got)
		}
	}
	if _, err := os.Stat(crestPath(second)); !os.IsNotExist(err) {
		t.Fatalf("deleted crest image: %v, want removed", err)
	}

	w.leader.Send(encodeSetAllyCrest(nil))
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("deleting an unset crest = %x, want nothing", opcodes(frames))
	}
}

// TestAllianceDissolutionClearsStaleCrest dissolves an alliance while
// Thirds, a clan in no alliance, still stores an alliance crest: the
// dissolution clears it as the reference's update by the zero alliance id
// does, and refreshes Third, who is not told of the dissolution. The
// stale crest's image is not Thirds' alliance's to remove and stays.
func TestAllianceDissolutionClearsStaleCrest(t *testing.T) {
	dir := t.TempDir()
	crests := datacache.NewCrestsIn(dir)
	const staleCrest = 7
	if err := crests.Save(datacache.AllyCrest, staleCrest, bytes.Repeat([]byte{0x33}, clientpackets.AllyCrestMaxLength)); err != nil {
		t.Fatalf("store the stale crest: %v", err)
	}
	w := bootAllianceCast(t, []castMember{third}, thirdsClan(`0, NULL, `+strconv.Itoa(staleCrest)), gameservertest.WithCrests(crests))
	if got := storedAlly(t, w, thirdsClanID); got.allyCrestID != staleCrest {
		t.Fatalf("Thirds' row at boot = %+v, want its stale crest kept", got)
	}
	w.formAlliance(t)
	th := w.bringIn(t, third)

	w.leader.Send(encodeAllyBare(clientpackets.OpcodeRequestDismissAlly))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	frames := drainFrames(t, th)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeUserInfo); !ok {
		t.Fatalf("Third after the dissolution = %x, want its UserInfo refreshed", opcodes(frames))
	}
	expectMessages(t, "Third after the dissolution", frames)
	if got := storedAlly(t, w, thirdsClanID); got.allyID != 0 || got.allyCrestID != 0 {
		t.Fatalf("Thirds' row after the dissolution = %+v, want its stale crest cleared", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "AllyCrest_"+strconv.Itoa(staleCrest)+".dds")); err != nil {
		t.Fatalf("stale crest image after the dissolution: %v, want kept", err)
	}
}
