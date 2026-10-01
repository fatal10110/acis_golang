package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRankPrivileges has the leader give rank 6 the invite and dismiss
// privileges: every member online gets its clan window refreshed and the
// rank's row is stored. The privilege window then reads them back, the
// rank counts and a member's rank card follow, and a rank 6 member may now
// invite. A rank set by anyone but the leader is ignored.
func TestRankPrivileges(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)
	privs := int32(clan.PrivInvite | clan.PrivDismiss)

	w.member.Send(encodeRequestPledgePower(6, 2, int32(clan.PrivAll)))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("member's privilege change = %x, want nothing", opcodes(frames))
	}

	w.leader.Send(encodeRequestPledgePower(6, 2, privs))
	refresh := []byte{serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo}
	for _, who := range []struct {
		name   string
		frames [][]byte
	}{{"leader", drainFrames(t, w.leader)}, {"recruit", drainFrames(t, w.member)}} {
		if got := opcodes(who.frames); string(got) != string(refresh) {
			t.Fatalf("%s's refresh = %x, want %x", who.name, got, refresh)
		}
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT privs FROM clan_privs WHERE clan_id = ? AND ranking = 6`, clanID); got != int64(privs) {
		t.Fatalf("stored rank 6 privileges = %d, want %d", got, privs)
	}

	w.member.Send(encodeRequestPledgePower(6, 1, 0))
	frames := drainFrames(t, w.member)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeManagePledgePower {
		t.Fatalf("privilege window = %x, want ManagePledgePower", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if rank, action, got := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); rank != 6 || action != 1 || got != privs {
		t.Fatalf("ManagePledgePower = %d %d %d, want 6 1 %d", rank, action, got, privs)
	}

	w.member.Send(encodeExtended(clientpackets.OpcodeRequestPledgePowerGrades).Bytes())
	frames = drainFrames(t, w.member)
	if len(frames) != 1 {
		t.Fatalf("rank counts = %x, want one packet", opcodes(frames))
	}
	r = wire.NewReader(frames[0][3:])
	if n := r.ReadInt32(); n != 9 {
		t.Fatalf("rank count entries = %d, want 9", n)
	}
	counts := map[int32]int32{}
	for range 9 {
		rank := r.ReadInt32()
		counts[rank] = r.ReadInt32()
	}
	if counts[6] != 1 || counts[1] != 0 {
		t.Fatalf("rank counts = %v, want one member at rank 6", counts)
	}

	w.leader.Send(encodeRequestPledgeMemberName(clientpackets.OpcodeRequestPledgeMemberPower, "Recruit"))
	frames = drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("rank card = %x, want one packet", opcodes(frames))
	}
	r = wire.NewReader(frames[0][3:])
	if grade, name, got := r.ReadInt32(), r.ReadString(), r.ReadInt32(); grade != 6 || name != "Recruit" || got != privs {
		t.Fatalf("rank card = %d %q %d, want 6 Recruit %d", grade, name, got, privs)
	}

	w.leader.Send(encodeRequestPledgeMemberName(clientpackets.OpcodeRequestPledgeMemberDetail, "Recruit"))
	frames = drainFrames(t, w.leader)
	if len(frames) != 1 {
		t.Fatalf("member card = %x, want one packet", opcodes(frames))
	}
	r = wire.NewReader(frames[0][3:])
	r.ReadInt32()
	if name, title, grade, pledge, mentor := r.ReadString(), r.ReadString(), r.ReadInt32(), r.ReadString(), r.ReadString(); name != "Recruit" || title != "" || grade != 6 || pledge != "Knights" || mentor != "" {
		t.Fatalf("member card = %q %q %d %q %q", name, title, grade, pledge, mentor)
	}

	// Rank 6 may now invite: the refusal is no longer about authority.
	w.member.Send(encodeRequestJoinPledge(w.leaderID, 0))
	frames = drainFrames(t, w.member)
	if len(frames) != 1 {
		t.Fatalf("rank 6 invitation = %x", opcodes(frames))
	}
	if id, _ := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1WorkingWithAnotherClan {
		t.Fatalf("rank 6 invitation message = %d, want S1_WORKING_WITH_ANOTHER_CLAN", id)
	}
}

// TestSetMemberGrade has the leader move the recruit to rank 5: every
// member online gets its row and the change notice, and the rank is
// stored. A member without the rank-management privilege is refused, and a
// grade outside 1-9 is ignored.
func TestSetMemberGrade(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.member.Send(encodeRequestPledgeSetMemberPowerGrade("Recruit", 1))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageNotAuthorizedToDoThat}) {
		t.Fatalf("member's own promotion = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
	}
	w.leader.Send(encodeRequestPledgeSetMemberPowerGrade("Recruit", 10))
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("grade 10 = %x, want nothing", opcodes(frames))
	}

	w.leader.Send(encodeRequestPledgeSetMemberPowerGrade("Recruit", 5))
	for _, who := range []struct {
		name   string
		frames [][]byte
	}{{"leader", drainFrames(t, w.leader)}, {"recruit", drainFrames(t, w.member)}} {
		if got := opcodes(who.frames); string(got) != string([]byte{serverpackets.OpcodePledgeShowMemberListUpdate, serverpackets.OpcodeSystemMessage}) {
			t.Fatalf("%s's grade notice = %x", who.name, got)
		}
		if id, params := sysMsg(t, who.frames[1]); id != serverpackets.SystemMessageClanMemberS1PrivilegeChangedToS2 || !slices.Equal(params, []string{"Recruit", "5"}) {
			t.Fatalf("%s's grade message = %d %v", who.name, id, params)
		}
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.memberID); got != 5 {
		t.Fatalf("stored power grade = %d, want 5", got)
	}
}
