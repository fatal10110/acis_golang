package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRestoreDialogAnswerAcceptsThePendingRevive pins DlgAnswer's routing:
// the DO_YOU_WANT_TO_BE_RESTORED question shares the resurrection offer's
// answer, so accepting it with that dialog id takes the pending revive. A
// dialog id that is none of the answered ones does nothing: the corpse stays
// dead and the offer stays open for its own id.
func TestRestoreDialogAnswerAcceptsThePendingRevive(t *testing.T) {
	t.Parallel()
	s := newResurrectionScene(t)

	s.castResurrection(t)
	if offer := indexOfOpcode(readQuiet(s.victim), 0, serverpackets.OpcodeConfirmDlg); offer < 0 {
		t.Fatal("victim never got the resurrection offer")
	}

	const unrelatedDialog = 1
	s.victim.Send(encodeDlgAnswer(unrelatedDialog, 1, 0))
	s.srv.Settle(t)
	if !s.v.Dead() {
		t.Fatal("an answer to an unrelated dialog revived the corpse")
	}
	if queued := s.victim.ReadQueued(); len(queued) != 0 {
		t.Fatalf("unrelated answer sent opcode %#x, want nothing", queued[0][0])
	}

	s.victim.Send(encodeDlgAnswer(serverpackets.ConfirmDlgRestoreRequest, 1, 0))
	s.srv.Settle(t)
	if s.v.Dead() {
		t.Fatal("accepting the restore dialog left the victim dead")
	}
	if i := indexOf(readQuiet(s.healer), 0, serverpackets.OpcodeRevive, s.victimID); i < 0 {
		t.Fatal("healer never saw the victim revive")
	}
	death, restored := deathAndRestore(t, 0, 100)
	if exp, _ := persistedExp(t, s.srv, s.victim, s.victimID); exp != death.After+restored {
		t.Fatalf("persisted exp = %d, want %d with the lost exp restored", exp, death.After+restored)
	}
}
