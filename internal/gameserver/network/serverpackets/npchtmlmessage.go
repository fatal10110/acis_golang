package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeNpcHtmlMessage is the wire opcode for NpcHtmlMessage.
const OpcodeNpcHtmlMessage = 0x0f

// FrameNpcHtmlMessage builds an HTML dialog packet.
func FrameNpcHtmlMessage(objectID int32, html string, itemID int32) wire.Frame {
	if len(html) > 8192 {
		html = "<html><body>Html was too long.</body></html>"
	}
	w := newFrameWriter(OpcodeNpcHtmlMessage)
	w.WriteInt32(objectID)
	w.WriteString(html)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
