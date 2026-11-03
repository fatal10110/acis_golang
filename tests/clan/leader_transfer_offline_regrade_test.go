package clan

import "testing"

// TestLeaderTransferOfflineFormerLeaderRegradedBeforeLogin: the founder
// nominates the recruit and logs out; the transfer leaves the founder shown
// at 0 with 6 stored. The new leader then gives the offline founder rank 3.
// That explicit grade replaces the transfer's: the clan sees 3 at once, 3 is
// stored, and the founder's next login keeps 3 instead of the transfer's 6
// (ClanMember.setPowerGrade caches and stores an offline member's grade,
// ClanMember.java:211-230; Player.restore reads it back, Player.java:4080).
func TestLeaderTransferOfflineFormerLeaderRegradedBeforeLogin(t *testing.T) {
	t.Parallel()
	w, witness := bootTransferWithWitness(t)
	nominateRecruit(t, w)
	w.leaveWorld(t, w.leader)
	drainFrames(t, w.member)
	drainFrames(t, witness)
	runTransfer(t, w)
	drainFrames(t, w.member)
	drainFrames(t, witness)
	wantShownGrade(t, witness, "after the transfer", "Founder", 0, 1)

	w.member.Send(encodeRequestPledgeSetMemberPowerGrade("Founder", 3))
	drainFrames(t, w.member)
	drainFrames(t, witness)
	wantShownGrade(t, witness, "after the regrade", "Founder", 3, 1)
	wantStoredGrades(t, w, "after the regrade", 3, 0)

	founder := w.srv.DialClient(t, "player1", 1)
	startInWorld(t, founder)
	drainFrames(t, founder)
	drainFrames(t, w.member)
	drainFrames(t, witness)
	wantShownGrade(t, witness, "after the founder's login", "Founder", 3, 1)
	wantStoredGrades(t, w, "after the founder's login", 3, 0)
}
