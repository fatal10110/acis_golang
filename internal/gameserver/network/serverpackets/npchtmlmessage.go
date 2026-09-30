package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeNpcHtmlMessage is the wire opcode for NpcHtmlMessage.
const OpcodeNpcHtmlMessage = 0x0f

// maxNpcHTML is the longest HTML an NpcHtmlMessage carries.
const maxNpcHTML = 8192

// NpcHtmlBody is html as an NpcHtmlMessage carries it: a page longer than
// the client accepts is replaced by a notice saying so.
func NpcHtmlBody(html string) string {
	if len(html) > maxNpcHTML {
		return "<html><body>Html was too long.</body></html>"
	}
	return html
}

// FrameNpcHtmlMessage builds an HTML dialog packet.
func FrameNpcHtmlMessage(objectID int32, html string, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeNpcHtmlMessage)
	w.WriteInt32(objectID)
	w.WriteString(NpcHtmlBody(html))
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
