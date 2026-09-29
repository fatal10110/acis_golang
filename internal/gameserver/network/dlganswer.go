package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleDlgAnswer routes a client's ConfirmDlg response by the dialog's
// message id: the resurrection offer (or the restore question that shares
// its answer) and the summon-friend request. Other message ids (engage,
// gate) have no dialog sender yet, so an answer for one of them finds no
// pending state and does nothing, as it would with no dialog open. The
// dialog itself closes client-side, so no answer is owed on any branch.
func (l *GameClientLink) handleDlgAnswer(live *livePlayer, req clientpackets.DlgAnswer) {
	switch req.MessageID {
	case serverpackets.ConfirmDlgResurrectionRequest, serverpackets.ConfirmDlgRestoreRequest:
		live.Character.ReviveAnswer(req.Answer)
	case serverpackets.ConfirmDlgSummonFriendRequest:
		live.Character.TeleportAnswer(req.Answer, req.RequesterID)
	}
}
