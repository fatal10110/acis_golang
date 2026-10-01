package serverpackets

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeConfirmDlg is the wire opcode for ConfirmDlg.
const OpcodeConfirmDlg = 0xed

// ConfirmDlgSummonFriendRequest is system message
// S1_WISHES_TO_SUMMON_YOU_FROM_S2_DO_YOU_ACCEPT, the messageId skill
// 1403's summon-confirm dialog carries and the id DlgAnswer's response
// echoes back.
const ConfirmDlgSummonFriendRequest int32 = 1842

// ConfirmDlgResurrectionRequest is SystemMessageId's
// RESSURECTION_REQUEST_BY_S1, the resurrection offer's messageId.
const ConfirmDlgResurrectionRequest int32 = 1510

// ConfirmDlgRestoreRequest is SystemMessageId's DO_YOU_WANT_TO_BE_RESTORED,
// whose answer is handled as a resurrection offer's.
const ConfirmDlgRestoreRequest int32 = 332

const (
	confirmDlgTypeText     = 0
	confirmDlgTypeZoneName = 7
)

// FrameConfirmDlgSummonFriendRequest builds skill 1403's summon-confirm
// dialog: messageId, the caster's name (TYPE_TEXT), the caster's position
// (TYPE_ZONE_NAME), the client-UI countdown, and the caster's object id for
// DlgAnswer's requesterId echo — ConfirmDlg's two-parameter layout followed
// by the countdown and requester id.
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

// FrameConfirmDlgResurrectionRequest builds the resurrection offer: the
// message id and the reviver's name as its single TYPE_TEXT parameter, with
// no countdown or requester id (ConfirmDlg.writeImpl writes neither when
// they are zero).
func FrameConfirmDlgResurrectionRequest(reviverName string) wire.Frame {
	w := newFrameWriter(OpcodeConfirmDlg)
	w.WriteInt32(ConfirmDlgResurrectionRequest)
	w.WriteInt32(1)
	w.WriteInt32(confirmDlgTypeText)
	w.WriteString(reviverName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
