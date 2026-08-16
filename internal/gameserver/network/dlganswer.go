package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleDlgAnswer routes a client's ConfirmDlg response, matching
// DlgAnswer.runImpl's messageId dispatch (DlgAnswer.java:20-37). Only the
// summon-confirm dialog is wired: other messageIds (revive, engage, gate)
// have no dialog sender yet, so a client answer for one of them is a no-op
// here exactly as it is in the reference when its own player.* method isn't
// reached — no pending state exists server-side for an unrecognized id.
func (l *GameClientLink) handleDlgAnswer(live *livePlayer, req clientpackets.DlgAnswer) {
	if req.MessageID != serverpackets.ConfirmDlgSummonFriendRequest {
		return
	}
	live.Character.TeleportAnswer(req.Answer, req.RequesterID)
}
