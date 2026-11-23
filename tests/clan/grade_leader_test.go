package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSetMemberGradeRefusesLeader (#5106): the leader keeps grade 0. Asking
// to regrade it is refused as not authorized, nobody else hears of it, and
// the roster and the stored grade stay as they were.
func TestSetMemberGradeRefusesLeader(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)
	w.srv.FlushPersistence(t)
	stored := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.leaderID)

	w.leader.Send(encodeRequestPledgeSetMemberPowerGrade("Founder", 5))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageNotAuthorizedToDoThat}) {
		t.Fatalf("leader's regrade = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
	}
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("recruit after the refused regrade = %x, want nothing", opcodes(frames))
	}
	cl, ok := w.srv.Clans.Table().Get(clanID)
	if !ok {
		t.Fatal("Knights is gone")
	}
	if m, _ := cl.Member(w.leaderID); m.PowerGrade != 0 {
		t.Fatalf("leader's roster grade = %d, want 0", m.PowerGrade)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.leaderID); got != stored {
		t.Fatalf("leader's stored grade = %d, want %d", got, stored)
	}
}
