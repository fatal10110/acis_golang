package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestFoundClanAtVillageMaster founds a clan through the village master's
// create_clan command: the refusals first, each answered by its own
// message, then the founding, which opens the clan window (the full roster
// with the founder as leader and only member), refreshes the founder's
// status and says so, and stores the clan and the founder's membership.
func TestFoundClanAtVillageMaster(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.talkToMaster(t)

	for _, tc := range []struct {
		name string
		want int
	}{
		{"Kn!ghts", serverpackets.SystemMessageClanNameInvalid},
		{"K", serverpackets.SystemMessageClanNameLengthIncorrect},
		{"Knights0123456789", serverpackets.SystemMessageClanNameLengthIncorrect},
	} {
		frames := w.masterCommand(t, "create_clan "+tc.name)
		if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
			t.Fatalf("create_clan %q answer = %x, want SystemMessage, ActionFailed", tc.name, got)
		}
		if id, _ := sysMsg(t, frames[0]); id != tc.want {
			t.Fatalf("create_clan %q message = %d, want %d", tc.name, id, tc.want)
		}
	}

	frames := w.masterCommand(t, "create_clan Knights")
	want := []byte{serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("create_clan answer = %x, want %x", got, want)
	}
	if id, _ := sysMsg(t, frames[2]); id != serverpackets.SystemMessageClanCreated {
		t.Fatalf("create_clan message = %d, want CLAN_CREATED", id)
	}
	r := wire.NewReader(frames[0][1:])
	if sub := r.ReadInt32(); sub != 0 {
		t.Fatalf("roster sub-unit flag = %d, want 0", sub)
	}
	clanID := r.ReadInt32()
	if pledgeType := r.ReadInt32(); pledgeType != 0 {
		t.Fatalf("roster pledge type = %d, want 0", pledgeType)
	}
	if name, leader := r.ReadString(), r.ReadString(); name != "Knights" || leader != "Founder" {
		t.Fatalf("roster header = %q led by %q, want Knights led by Founder", name, leader)
	}
	for range 9 { // crest, level, castle, hall, rank, reputation, dissolution, 0, ally
		r.ReadInt32()
	}
	r.ReadString() // ally name
	r.ReadInt32()  // ally crest
	r.ReadInt32()  // at war
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("roster count = %d, want 1", count)
	}
	if name := r.ReadString(); name != "Founder" {
		t.Fatalf("roster row = %q, want Founder", name)
	}
	r.ReadInt32() // level
	r.ReadInt32() // class
	r.ReadInt32() // sex
	r.ReadInt32() // race
	if online := r.ReadInt32(); online != w.leaderID {
		t.Fatalf("founder row online id = %d, want %d", online, w.leaderID)
	}

	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT leader_id FROM clan_data WHERE clan_id = ? AND clan_name = 'Knights' AND clan_level = 0`, clanID); got != int64(w.leaderID) {
		t.Fatalf("clan_data leader = %d, want %d", got, w.leaderID)
	}
	if got := queryInt(t, w, `SELECT clanid FROM characters WHERE obj_Id = ? AND power_grade = 0 AND subpledge = 0`, w.leaderID); got != int64(clanID) {
		t.Fatalf("founder clanid = %d, want %d", got, clanID)
	}

	// A member founds no second clan.
	frames = w.masterCommand(t, "create_clan Templars")
	if ids := messages(t, frames); !slices.Equal(ids, []int{serverpackets.SystemMessageFailedToCreateClan}) {
		t.Fatalf("second founding messages = %v, want FAILED_TO_CREATE_CLAN", ids)
	}
}

// TestFoundClanRefusesTakenNameAndLowLevel refuses a name another clan
// carries, ignoring case, and a founder below level 10.
func TestFoundClanRefusesTakenNameAndLowLevel(t *testing.T) {
	w := bootClanWorld(t, 9, 0, 0)
	w.talkToMaster(t)
	frames := w.masterCommand(t, "create_clan Knights")
	if ids := messages(t, frames); !slices.Equal(ids, []int{serverpackets.SystemMessageNotMeetCriteriaToCreateClan}) {
		t.Fatalf("level 9 founding messages = %v, want YOU_DO_NOT_MEET_CRITERIA_IN_ORDER_TO_CREATE_A_CLAN", ids)
	}

	w.srv.AddPlayerLevel(t, w.leaderID, 1)
	drainFrames(t, w.leader)
	w.found(t, "Knights")

	// The recruit, raised to level 10, takes the master's page too.
	w.srv.AddPlayerLevel(t, w.memberID, 9)
	drainFrames(t, w.member)
	w.member.Send(encodeAction(w.master.ObjectID(), w.at))
	drainFrames(t, w.member)
	w.member.Send(encodeAction(w.master.ObjectID(), w.at))
	drainFrames(t, w.member)
	w.member.Send(encodeBypass("npc_" + itoa(w.master.ObjectID()) + "_create_clan KNIGHTS"))
	frames = drainFrames(t, w.member)
	frame, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage)
	if !ok {
		t.Fatalf("taken-name founding answer = %x, want S1_ALREADY_EXISTS", opcodes(frames))
	}
	if id, params := sysMsg(t, frame); id != serverpackets.SystemMessageS1AlreadyExists || !slices.Equal(params, []string{"KNIGHTS"}) {
		t.Fatalf("taken-name founding message = %d %v, want S1_ALREADY_EXISTS KNIGHTS", id, params)
	}
}
