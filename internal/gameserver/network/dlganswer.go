package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleDlgAnswer routes a client's ConfirmDlg response by the dialog's
// message id: the resurrection offer (or the restore question that shares
// its answer), the summon-friend request, the marriage request and a clan
// hall door's open or close question. An answer finding no pending state
// does nothing, as it would with no dialog open. The dialog itself closes
// client-side, so no answer is owed on any branch.
func (l *GameClientLink) handleDlgAnswer(live *livePlayer, req clientpackets.DlgAnswer) {
	switch req.MessageID {
	case serverpackets.ConfirmDlgResurrectionRequest, serverpackets.ConfirmDlgRestoreRequest:
		live.Character.ReviveAnswer(req.Answer)
	case serverpackets.ConfirmDlgSummonFriendRequest:
		live.Character.TeleportAnswer(req.Answer, req.RequesterID)
	case serverpackets.ConfirmDlgEngageRequest:
		l.engageAnswer(live, req.Answer)
	case serverpackets.ConfirmDlgOpenGate:
		l.answerGate(live, req.Answer, true)
	case serverpackets.ConfirmDlgCloseGate:
		l.answerGate(live, req.Answer, false)
	}
}
