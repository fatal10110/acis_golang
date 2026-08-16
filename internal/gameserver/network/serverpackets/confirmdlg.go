package serverpackets

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeConfirmDlg is the wire opcode for ConfirmDlg.
const OpcodeConfirmDlg = 0xed

// ConfirmDlgSummonFriendRequest is SystemMessageId's
// S1_WISHES_TO_SUMMON_YOU_FROM_S2_DO_YOU_ACCEPT, the messageId skill
// 1403's summon-confirm dialog carries (SummonFriend.java:78,
// SystemMessageId.java:13579) and the id DlgAnswer's response echoes back.
const ConfirmDlgSummonFriendRequest int32 = 1842

const confirmDlgTypeText = 0
const confirmDlgTypeZoneName = 7

// FrameConfirmDlgSummonFriendRequest builds skill 1403's summon-confirm
// dialog: messageId, the caster's name (TYPE_TEXT), the caster's position
// (TYPE_ZONE_NAME), the client-UI countdown, and the caster's object id for
// DlgAnswer's requesterId echo — matching ConfirmDlg.writeImpl's two-entry
// branch (ConfirmDlg.java:113-159) as built by
// SummonFriend.java:76-84 (addCharName/addZoneName/addTime/addRequesterId).
func FrameConfirmDlgSummonFriendRequest(casterName string, casterID int32, x, y, z int32, timeout time.Duration) wire.Frame {
	w := newFrameWriter(OpcodeConfirmDlg)
	w.WriteInt32(ConfirmDlgSummonFriendRequest)
	w.WriteInt32(2)
	w.WriteInt32(confirmDlgTypeText)
	w.WriteString(casterName)
	w.WriteInt32(confirmDlgTypeZoneName)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(int32(timeout / time.Millisecond))
	w.WriteInt32(casterID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
