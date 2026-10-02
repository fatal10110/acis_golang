package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// academySeed is the seeded clan's academy, "School".
var academySeed = `INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (` + itoa(subunitClanID) + `, -1, 'School', 0)`

// subPledgeCreated decodes a PledgeReceiveSubPledgeCreated frame.
func subPledgeCreated(t *testing.T, frames [][]byte) (pledgeType int32, name, leader string) {
	t.Helper()
	created := extended(frames, serverpackets.OpcodeExPledgeReceiveSubPledgeCreate)
	if len(created) != 1 {
		t.Fatalf("sub-unit notices = %d in %x, want 1", len(created), opcodes(frames))
	}
	r := wire.NewReader(created[0][3:])
	r.ReadInt32()
	return r.ReadInt32(), r.ReadString(), r.ReadString()
}

// wantMessage fails unless frames carry exactly one system message, id
// with params.
func wantMessage(t *testing.T, what string, frames [][]byte, id int, params ...string) {
	t.Helper()
	var got []int
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		gotID, gotParams := sysMsg(t, f)
		got = append(got, gotID)
		if gotID == id && len(params) > 0 && !slices.Equal(gotParams, params) {
			t.Fatalf("%s params = %v, want %v", what, gotParams, params)
		}
	}
	if !slices.Equal(got, []int{id}) {
		t.Fatalf("%s messages = %v, want [%d]", what, got, id)
	}
}

// TestSubunitFoundings founds an academy, a royal guard and a knight order
// through the village master, with every refusal on the way, then renames
// one and staffs another. Each founding shows the clan its header and the
// new sub-unit, a military one costs reputation, and the rows are stored.
func TestSubunitFoundings(t *testing.T) {
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t))
	w.recruit(t)

	leader := w.masterCommandBy(t, w.leader, "create_academy School")
	recruit := drainFrames(t, w.member)
	if got, want := only(leader, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeExtended, serverpackets.OpcodeSystemMessage),
		[]byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeExtended, serverpackets.OpcodeSystemMessage}; !slices.Equal(got, want) {
		t.Fatalf("academy founding = %x, want %x", got, want)
	}
	if id, name, captain := subPledgeCreated(t, leader); id != -1 || name != "School" || captain != "" {
		t.Fatalf("academy notice = %d %q %q", id, name, captain)
	}
	wantMessage(t, "academy founding", leader, serverpackets.SystemMessageS1ClanAcademyCreated, "Knights")
	if id, _, _ := subPledgeCreated(t, recruit); id != -1 {
		t.Fatalf("recruit's academy notice = %d", id)
	}

	for _, c := range []struct {
		command string
		id      int
		params  []string
	}{
		{"create_academy school", serverpackets.SystemMessageS1AlreadyExists, []string{"school"}},
		{"create_academy Other", serverpackets.SystemMessageClanAlreadyHasAcademy, nil},
		{"create_royal Guards Founder", serverpackets.SystemMessageNotMeetCriteriaForMilitaryUnit, nil},
		{"create_royal Guards Nobody", serverpackets.SystemMessageRoyalCaptainCannotBeAppointed, nil},
		{"create_royal Guards", serverpackets.SystemMessageRoyalCaptainCannotBeAppointed, nil},
		{"create_royal G! Squire", serverpackets.SystemMessageClanNameInvalid, nil},
		{"create_knight Order Nobody", serverpackets.SystemMessageKnightCaptainCannotBeAppointed, nil},
	} {
		wantMessage(t, c.command, w.masterCommandBy(t, w.leader, c.command), c.id, c.params...)
	}

	wantMessage(t, "rename of a missing unit", w.masterCommandBy(t, w.leader, "rename_pledge 100 Other"), serverpackets.SystemMessageS1, "Pledge doesn't exist.")
	leader = w.masterCommandBy(t, w.leader, "create_royal Guards Squire")
	if got := only(leader, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeExtended); !slices.Equal(got,
		[]byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeExtended}) {
		t.Fatalf("royal founding = %x, want the reputation's header, then the header and notice", got)
	}
	if id, name, captain := subPledgeCreated(t, leader); id != 100 || name != "Guards" || captain != "Squire" {
		t.Fatalf("royal notice = %d %q %q", id, name, captain)
	}
	wantMessage(t, "royal founding", leader, serverpackets.SystemMessageRoyalGuardOfS1Created, "Knights")
	leader = w.masterCommandBy(t, w.leader, "create_knight Order Squire")
	if id, name, _ := subPledgeCreated(t, leader); id != 1001 || name != "Order" {
		t.Fatalf("knight notice = %d %q", id, name)
	}
	wantMessage(t, "knight founding", leader, serverpackets.SystemMessageKnightsOfS1Created, "Knights")
	wantMessage(t, "taken military name", w.masterCommandBy(t, w.leader, "create_knight guards Squire"), serverpackets.SystemMessageMilitaryUnitNameTaken)
	wantMessage(t, "knight past the reputation", w.masterCommandBy(t, w.leader, "create_knight Order2 Squire"), serverpackets.SystemMessageClanReputationScoreTooLow)
	drainFrames(t, w.member)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, subunitClanID); got != 5000 {
		t.Fatalf("stored reputation = %d, want 20000 less 5000 and 10000", got)
	}
	if got := queryInt(t, w, `SELECT leader_id FROM clan_subpledges WHERE clan_id = ? AND sub_pledge_id = 100 AND name = 'Guards'`, subunitClanID); got != int64(squireID) {
		t.Fatalf("stored royal captain = %d, want %d", got, squireID)
	}

	w.masterCommandBy(t, w.leader, "rename_pledge 100 Wardens")
	frames := drainFrames(t, w.member)
	list, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("rename answer to the recruit = %x", opcodes(frames))
	}
	if id, name, captain, _ := unitList(t, list); id != 100 || name != "Wardens" || captain != "Squire" {
		t.Fatalf("renamed roster = %d %q %q", id, name, captain)
	}

	for _, c := range []struct {
		command string
		id      int
	}{
		{"assign_subpl_leader Wardens Founder", serverpackets.SystemMessageRoyalCaptainCannotBeAppointed},
		{"assign_subpl_leader School Recruit", serverpackets.SystemMessageClanNameInvalid},
		{"assign_subpl_leader Wardens Squire", serverpackets.SystemMessageRoyalCaptainCannotBeAppointed},
		{"assign_subpl_leader Order Nobody", serverpackets.SystemMessageKnightCaptainCannotBeAppointed},
		{"assign_subpl_leader Order Seventeen_letters", serverpackets.SystemMessageNamingCharnameUpTo16Chars},
	} {
		wantMessage(t, c.command, w.masterCommandBy(t, w.leader, c.command), c.id)
	}
	leader = w.masterCommandBy(t, w.leader, "assign_subpl_leader order Recruit")
	list, ok = firstOpcode(leader, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("captaincy answer = %x", opcodes(leader))
	}
	if id, _, captain, _ := unitList(t, list); id != 1001 || captain != "Recruit" {
		t.Fatalf("knight roster = %d %q, want Recruit captaining 1001", id, captain)
	}
	wantMessage(t, "captaincy", leader, serverpackets.SystemMessageS1SelectedAsCaptainOfS2, "Recruit", "order")
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("new captain not shown its status")
	}

	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	if got := unitLists(t, drainFrames(t, w.leader)); !slices.Equal(got, []int32{0, -1, 100, 1001}) {
		t.Fatalf("rosters = %v, want main, academy, royal guard, knights", got)
	}
}

// TestAcademyJoinAndMentor recruits into the academy, links the recruit
// to the leader as apprentice and sponsor and unlinks them, then moves the
// recruit between sub-units. An invitation into a sub-unit the clan has not
// founded is refused.
func TestAcademyJoinAndMentor(t *testing.T) {
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t, academySeed))

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 100))
	wantMessage(t, "invitation into an unfounded unit", drainFrames(t, w.leader), serverpackets.SystemMessageSubclanIsFull)

	w.leader.Send(encodeRequestJoinPledge(w.memberID, -1))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	frames := drainFrames(t, w.member)
	if got := unitLists(t, frames); !slices.Equal(got, []int32{0, -1}) {
		t.Fatalf("recruit's rosters = %v, want main then academy", got)
	}
	for _, f := range frames {
		if f[0] != serverpackets.OpcodePledgeShowMemberListAll {
			continue
		}
		if id, name, _, members := unitList(t, f); id == -1 && (name != "School" || !slices.Equal(members, []string{"Recruit"})) {
			t.Fatalf("academy roster = %q %v", name, members)
		}
	}
	drainFrames(t, w.leader)
	w.srv.FlushPersistence(t)
	for column, want := range map[string]int64{"subpledge": -1, "power_grade": 9, "lvl_joined_academy": 1} {
		if got := queryInt(t, w, `SELECT `+column+` FROM characters WHERE obj_Id = ?`, w.memberID); got != want {
			t.Fatalf("stored %s = %d, want %d", column, got, want)
		}
	}

	w.leader.Send(encodeRequestPledgeSetAcademyMaster(1, "Recruit", "Founder"))
	leader, recruit := drainFrames(t, w.leader), drainFrames(t, w.member)
	if got := only(leader, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate); !slices.Equal(got,
		[]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate, serverpackets.OpcodePledgeShowMemberListUpdate}) {
		t.Fatalf("sponsor's answer = %x", got)
	}
	wantMessage(t, "link", leader, serverpackets.SystemMessageS2DesignatedApprenticeOfS1, "Founder", "Recruit")
	wantMessage(t, "apprentice's link", recruit, serverpackets.SystemMessageS2DesignatedApprenticeOfS1, "Founder", "Recruit")
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT sponsor FROM characters WHERE obj_Id = ?`, w.memberID); got != int64(w.leaderID) {
		t.Fatalf("stored sponsor = %d, want %d", got, w.leaderID)
	}
	if got := queryInt(t, w, `SELECT apprentice FROM characters WHERE obj_Id = ?`, w.leaderID); got != int64(w.memberID) {
		t.Fatalf("stored apprentice = %d, want %d", got, w.memberID)
	}
	w.leader.Send(encodeRequestPledgeSetAcademyMaster(1, "Recruit", "Founder"))
	wantMessage(t, "second link", drainFrames(t, w.leader), serverpackets.SystemMessageS1, "Remove previous connections first.")

	w.leader.Send(encodeRequestPledgeMemberName(clientpackets.OpcodeRequestPledgeMemberDetail, "Recruit"))
	card := extended(drainFrames(t, w.leader), serverpackets.OpcodeExPledgeReceiveMemberInfo)
	if len(card) != 1 {
		t.Fatal("no member card")
	}
	r := wire.NewReader(card[0][3:])
	if pledgeType, name, _, grade, unit, mentor := r.ReadInt32(), r.ReadString(), r.ReadString(), r.ReadInt32(), r.ReadString(), r.ReadString(); pledgeType != -1 || name != "Recruit" || grade != 9 || unit != "School" || mentor != "Founder" {
		t.Fatalf("member card = %d %q %d %q %q", pledgeType, name, grade, unit, mentor)
	}

	w.leader.Send(encodeRequestPledgeSetAcademyMaster(0, "Founder", "Recruit"))
	wantMessage(t, "unlink", drainFrames(t, w.leader), serverpackets.SystemMessageS2ApprenticeOfS1Removed, "Founder", "Recruit")
	drainFrames(t, w.member)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT SUM(sponsor + apprentice) FROM characters WHERE obj_Id IN (?, ?)`, w.memberID, w.leaderID); got != 0 {
		t.Fatalf("stored links after unlink = %d, want 0", got)
	}

	w.member.Send(encodeRequestPledgeReorganizeMember(1, "Recruit", 0, "Squire"))
	wantMessage(t, "reorganization without the privilege", drainFrames(t, w.member), serverpackets.SystemMessageNotAuthorizedToDoThat)
	for _, req := range [][]byte{
		encodeRequestPledgeReorganizeMember(0, "Recruit", 0, ""),
		encodeRequestPledgeReorganizeMember(1, "Recruit", 200, "Squire"),
	} {
		w.leader.Send(req)
		frames := drainFrames(t, w.leader)
		if len(frames) != 1 || len(extended(frames, serverpackets.OpcodeExPledgeReceiveMemberInfo)) != 1 {
			t.Fatalf("reorganization answer = %x, want the recruit's card", opcodes(frames))
		}
	}
	w.leader.Send(encodeRequestPledgeReorganizeMember(1, "Recruit", 0, "Squire"))
	if got := only(drainFrames(t, w.leader), serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo); !slices.Equal(got,
		[]byte{serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo}) {
		t.Fatalf("reorganization refresh = %x", got)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT subpledge FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("stored recruit sub-unit = %d, want 0", got)
	}
	if got := queryInt(t, w, `SELECT subpledge FROM characters WHERE obj_Id = ?`, squireID); got != -1 {
		t.Fatalf("stored squire sub-unit = %d, want -1", got)
	}
}

// TestAcademyRequirements refuses an academy invitation of a level 41
// player with both academy notices.
func TestAcademyRequirements(t *testing.T) {
	w, _ := bootSubunitWorld(t, 41, seedSubunitClan(t, academySeed))
	w.leader.Send(encodeRequestJoinPledge(w.memberID, -1))
	frames := drainFrames(t, w.leader)
	if got := messages(t, frames); !slices.Equal(got, []int{serverpackets.SystemMessageS1NotMeetAcademyRequirements, serverpackets.SystemMessageAcademyRequirements}) {
		t.Fatalf("refusal = %v", got)
	}
	if _, params := sysMsg(t, frames[0]); !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("refusal names %v", params)
	}
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("refused recruit got %x", opcodes(frames))
	}
}

// TestLoginListsSubunits sends a sub-unit's roster after the main clan's
// in the login burst, academy first, then the knights of the second royal
// guard, then the first royal guard, whose captain it names. Expelling the
// captain leaves the post vacant.
func TestLoginListsSubunits(t *testing.T) {
	w, burst := bootSubunitWorld(t, 1, seedSubunitClan(t, academySeed,
		`INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (`+itoa(subunitClanID)+`, 100, 'Guards', `+itoa(squireID)+`)`,
		`INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (`+itoa(subunitClanID)+`, 2001, 'Order', 0)`))
	if got := unitLists(t, burst); !slices.Equal(got, []int32{0, -1, 2001, 100}) {
		t.Fatalf("login rosters = %v, want main, academy, 2001, 100", got)
	}
	for _, f := range burst {
		if f[0] == serverpackets.OpcodePledgeShowMemberListAll {
			if id, name, captain, _ := unitList(t, f); id == 100 && (name != "Guards" || captain != "Squire") {
				t.Fatalf("royal roster = %q %q", name, captain)
			}
		}
	}

	w.leader.Send(encodeRequestOustPledgeMember("Squire"))
	drainFrames(t, w.leader)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT leader_id FROM clan_subpledges WHERE clan_id = ? AND sub_pledge_id = 100`, subunitClanID); got != 0 {
		t.Fatalf("stored captain after expulsion = %d, want 0", got)
	}
	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	for _, f := range drainFrames(t, w.leader) {
		if f[0] == serverpackets.OpcodePledgeShowMemberListAll {
			if id, _, captain, _ := unitList(t, f); id == 100 && captain != "" {
				t.Fatalf("royal captain after expulsion = %q, want none", captain)
			}
		}
	}
}

// TestCaptainOnLoadingScreenIsNotRefreshed founds a royal guard whose
// captain has selected its character but not entered the world. The
// captain is not an online clan member until it enters, so the founding
// shows its client nothing on the loading screen.
func TestCaptainOnLoadingScreenIsNotRefreshed(t *testing.T) {
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t))
	squire := w.srv.DialClient(t, "squire", 1)
	squire.Send(encodeRequestGameStart(0))
	if reply := squire.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := squire.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	w.srv.AwaitHandled(t)

	leader := w.masterCommandBy(t, w.leader, "create_royal Guards Squire")
	if id, _, captain := subPledgeCreated(t, leader); id != 100 || captain != "Squire" {
		t.Fatalf("royal notice = %d %q, want Squire captaining 100", id, captain)
	}
	w.srv.Settle(t)
	if frames := drainFrames(t, squire); len(frames) != 0 {
		t.Fatalf("captain on its loading screen got %x, want nothing", opcodes(frames))
	}
}
