package clan

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A leader transfer gives an online former leader or nominee its new
// power grade at once, but leaves an offline one's roster entry at its old
// grade (Clan.setNewLeader only sets the online player's grade,
// Clan.java:340,349; ClanMember.getPowerGrade reads the cached grade of an
// offline member, ClanMember.java:206-209). Both grades are stored
// (Clan.java:359-372), and the stored grade shows once that member logs in
// again (Player.restore reads power_grade; EnterWorld links the roster
// entry to the player).

// bootTransferWithWitness is bootLeaderTransfer with a third member,
// Witness, in the world and the clan's main unit; it returns Witness's
// client. The clan clock moves past the founder's first invitation so the
// founder may invite again.
func bootTransferWithWitness(t *testing.T) (*clanWorld, *testsupport.ScriptedClient) {
	t.Helper()
	var now atomic.Int64
	now.Store(time.Now().UnixMilli())
	w, _ := bootLeaderTransfer(t, gameservertest.WithClanClock(func() time.Time { return time.UnixMilli(now.Load()) }))
	id := w.srv.SeedCharacterFor(t, "player3", "Witness", 40, 0).ID
	c := w.srv.DialClient(t, "player3", 1)
	startInWorld(t, c)
	drainFrames(t, c)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	now.Add(clan.InviteTimeout.Milliseconds())
	w.leader.Send(encodeRequestJoinPledge(id, 0))
	drainFrames(t, c)
	c.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Witness did not join")
	}
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return w, c
}

// cardGrade is the power grade c's member card for name shows.
func cardGrade(t *testing.T, c *testsupport.ScriptedClient, name string) int32 {
	t.Helper()
	c.Send(encodeRequestPledgeMemberName(clientpackets.OpcodeRequestPledgeMemberDetail, name))
	card := extended(drainFrames(t, c), serverpackets.OpcodeExPledgeReceiveMemberInfo)
	if len(card) != 1 {
		t.Fatalf("member card of %s = %d packets, want one", name, len(card))
	}
	r := wire.NewReader(card[0][3:])
	r.ReadInt32()
	if got := r.ReadString(); got != name {
		t.Fatalf("member card name = %q, want %q", got, name)
	}
	r.ReadString()
	return r.ReadInt32()
}

// rankInfoGrade is the power grade c's rank card for name shows.
func rankInfoGrade(t *testing.T, c *testsupport.ScriptedClient, name string) int32 {
	t.Helper()
	c.Send(encodeRequestPledgeMemberName(clientpackets.OpcodeRequestPledgeMemberPower, name))
	frames := drainFrames(t, c)
	if len(frames) != 1 {
		t.Fatalf("rank card of %s = %x, want one packet", name, opcodes(frames))
	}
	return wire.NewReader(frames[0][3:]).ReadInt32()
}

// gradeCounts is the member count of each rank 1-9 c's rank list shows.
func gradeCounts(t *testing.T, c *testsupport.ScriptedClient) map[int32]int32 {
	t.Helper()
	c.Send(encodeExtended(clientpackets.OpcodeRequestPledgePowerGrades).Bytes())
	frames := drainFrames(t, c)
	if len(frames) != 1 {
		t.Fatalf("rank counts = %x, want one packet", opcodes(frames))
	}
	r := wire.NewReader(frames[0][3:])
	counts := map[int32]int32{}
	for n := r.ReadInt32(); n > 0; n-- {
		rank := r.ReadInt32()
		counts[rank] = r.ReadInt32()
	}
	return counts
}

// wantShownGrade checks Witness's member card, rank card and rank list
// show name at grade, with sixes members at rank 6.
func wantShownGrade(t *testing.T, witness *testsupport.ScriptedClient, step, name string, grade, sixes int32) {
	t.Helper()
	if got := cardGrade(t, witness, name); got != grade {
		t.Errorf("%s: %s's member card grade = %d, want %d", step, name, got, grade)
	}
	if got := rankInfoGrade(t, witness, name); got != grade {
		t.Errorf("%s: %s's rank card grade = %d, want %d", step, name, got, grade)
	}
	if got := gradeCounts(t, witness)[6]; got != sixes {
		t.Errorf("%s: rank 6 count = %d, want %d", step, got, sixes)
	}
}

// wantStoredGrades checks the stored power grades of the founder and the
// recruit.
func wantStoredGrades(t *testing.T, w *clanWorld, step string, founder, recruit int64) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.leaderID); got != founder {
		t.Errorf("%s: founder's stored grade = %d, want %d", step, got, founder)
	}
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.memberID); got != recruit {
		t.Errorf("%s: recruit's stored grade = %d, want %d", step, got, recruit)
	}
}

// TestLeaderTransferOfflineFormerLeaderKeepsShownGrade: the founder
// nominates the recruit and logs out before the transfer. The clan still
// sees the founder at grade 0 until it logs in again, then at 6; the
// online recruit shows 0 at once. Stored grades are 6 and 0 throughout.
func TestLeaderTransferOfflineFormerLeaderKeepsShownGrade(t *testing.T) {
	t.Parallel()
	w, witness := bootTransferWithWitness(t)
	nominateRecruit(t, w)
	w.leaveWorld(t, w.leader)
	drainFrames(t, w.member)
	drainFrames(t, witness)
	runTransfer(t, w)
	drainFrames(t, w.member)
	drainFrames(t, witness)

	wantStoredGrades(t, w, "after the transfer", 6, 0)
	wantShownGrade(t, witness, "after the transfer", "Founder", 0, 1)
	if got := cardGrade(t, witness, "Recruit"); got != 0 {
		t.Errorf("after the transfer: online new leader's card grade = %d, want 0", got)
	}

	founder := w.srv.DialClient(t, "player1", 1)
	startInWorld(t, founder)
	drainFrames(t, founder)
	drainFrames(t, w.member)
	drainFrames(t, witness)
	wantShownGrade(t, witness, "after the founder's login", "Founder", 6, 2)
	wantStoredGrades(t, w, "after the founder's login", 6, 0)
}

// TestLeaderTransferOfflineNomineeKeepsShownGrade: the nominee is out of
// the world when the transfer runs. The clan still sees it at grade 6
// until it logs in again, then at 0; the online founder shows 6 at once.
func TestLeaderTransferOfflineNomineeKeepsShownGrade(t *testing.T) {
	t.Parallel()
	w, witness := bootTransferWithWitness(t)
	nominateRecruit(t, w)
	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, witness)
	runTransfer(t, w)
	drainFrames(t, w.leader)
	drainFrames(t, witness)

	wantStoredGrades(t, w, "after the transfer", 6, 0)
	wantShownGrade(t, witness, "after the transfer", "Recruit", 6, 3)
	if got := cardGrade(t, witness, "Founder"); got != 6 {
		t.Errorf("after the transfer: online former leader's card grade = %d, want 6", got)
	}

	recruit := w.srv.DialClient(t, "player2", 1)
	startInWorld(t, recruit)
	drainFrames(t, recruit)
	drainFrames(t, w.leader)
	drainFrames(t, witness)
	wantShownGrade(t, witness, "after the nominee's login", "Recruit", 0, 2)
	wantStoredGrades(t, w, "after the nominee's login", 6, 0)
}
