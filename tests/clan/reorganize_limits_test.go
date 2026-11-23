package clan

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// captainID is the seeded main-clan member captaining the royal guard;
// lanceBaseID+i the seeded knights filling the knight order.
const (
	captainID   int32 = 0x7f100101
	lanceBaseID int32 = 0x7f100200
)

// reorganizeLimitSeeds gives the seeded clan an empty academy, a royal
// guard captained by Captain (a main-clan member) and a knight order
// already holding its ten members.
func reorganizeLimitSeeds() []string {
	clanID := itoa(subunitClanID)
	stmts := []string{
		academySeed,
		`INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (` + clanID + `, ` + strconv.Itoa(clan.SubunitRoyal1) + `, 'Guard', ` + itoa(captainID) + `)`,
		`INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (` + clanID + `, ` + strconv.Itoa(clan.SubunitKnight1) + `, 'Lances', 0)`,
		`INSERT INTO characters (account_name, obj_Id, char_name, level, classid, clanid, power_grade, subpledge, sex, race)
			VALUES ('captain', ` + itoa(captainID) + `, 'Captain', 20, 0, ` + clanID + `, 6, 0, 0, 0)`,
	}
	for i := range int32(clan.MaxMembers(7, clan.SubunitKnight1)) {
		id := lanceBaseID + i
		stmts = append(stmts, `INSERT INTO characters (account_name, obj_Id, char_name, level, classid, clanid, power_grade, subpledge, sex, race)
			VALUES ('lance`+itoa(i)+`', `+itoa(id)+`, 'Lance`+itoa(i)+`', 20, 0, `+clanID+`, 7, `+strconv.Itoa(clan.SubunitKnight1)+`, 0, 0)`)
	}
	return stmts
}

// TestReorganizeRefusesFullUnitAndCaptains (#5106): with Recruit in the
// academy, a swap that would put an eleventh member into the full knight
// order, and one that would move the royal guard's captain into the
// academy, are each refused with Recruit's card and change no row. A true
// swap with a knight of the full order still goes through.
func TestReorganizeRefusesFullUnitAndCaptains(t *testing.T) {
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t, reorganizeLimitSeeds()...))
	w.leader.Send(encodeRequestJoinPledge(w.memberID, clan.SubunitAcademy))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("recruit did not join the academy")
	}
	drainFrames(t, w.leader)

	stored := func(id int32) int64 {
		t.Helper()
		w.srv.FlushPersistence(t)
		return queryInt(t, w, `SELECT subpledge FROM characters WHERE obj_Id = ?`, id)
	}
	for _, tc := range []struct {
		name    string
		unit    int32
		partner string
		id      int32
		unitOf  int64
	}{
		{"swap into the full knight order", clan.SubunitKnight1, "Squire", squireID, clan.SubunitMain},
		{"swap moving the captain out", clan.SubunitMain, "Captain", captainID, clan.SubunitMain},
	} {
		w.leader.Send(encodeRequestPledgeReorganizeMember(1, "Recruit", tc.unit, tc.partner))
		frames := drainFrames(t, w.leader)
		if len(frames) != 1 || len(extended(frames, serverpackets.OpcodeExPledgeReceiveMemberInfo)) != 1 {
			t.Fatalf("%s: answer = %x, want the recruit's card", tc.name, opcodes(frames))
		}
		if frames := drainFrames(t, w.member); len(frames) != 0 {
			t.Fatalf("%s: recruit got %x, want nothing", tc.name, opcodes(frames))
		}
		if got := stored(w.memberID); got != clan.SubunitAcademy {
			t.Fatalf("%s: recruit's stored sub-unit = %d, want the academy", tc.name, got)
		}
		if got := stored(tc.id); got != tc.unitOf {
			t.Fatalf("%s: %s's stored sub-unit = %d, want %d", tc.name, tc.partner, got, tc.unitOf)
		}
	}

	w.leader.Send(encodeRequestPledgeReorganizeMember(1, "Recruit", clan.SubunitKnight1, "Lance0"))
	if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("a true swap with a knight was not shown")
	}
	drainFrames(t, w.member)
	if got := stored(w.memberID); got != clan.SubunitKnight1 {
		t.Fatalf("recruit's stored sub-unit after the swap = %d, want %d", got, clan.SubunitKnight1)
	}
	if got := stored(lanceBaseID); got != clan.SubunitAcademy {
		t.Fatalf("Lance0's stored sub-unit after the swap = %d, want the academy", got)
	}
}
