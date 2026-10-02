package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeNpcHtmlMessage is the wire opcode for NpcHtmlMessage.
const OpcodeNpcHtmlMessage = 0x0f

// maxNpcHTML is the longest page, in UTF-16 code units, an NpcHtmlMessage
// takes when it is set.
const maxNpcHTML = 8192

// NpcHtmlBody is html as an NpcHtmlMessage takes it when the page is set: a
// page longer than 8192 UTF-16 code units is replaced by a notice saying
// so. It applies to a page as loaded or built, before its placeholders are
// filled: a page its fillings grow past the limit is still sent whole.
func NpcHtmlBody(html string) string {
	// A UTF-8 string holds at least as many bytes as UTF-16 code units.
	if len(html) <= maxNpcHTML {
		return html
	}
	units := 0
	for _, r := range html {
		// A rune past the Basic Multilingual Plane takes a surrogate pair.
		units++
		if r >= 0x10000 {
			units++
		}
		if units > maxNpcHTML {
			return "<html><body>Html was too long.</body></html>"
		}
	}
	return html
}

// FrameNpcHtmlMessage builds an HTML dialog packet carrying html as given;
// the page limit is applied where the page is set, by NpcHtmlBody.
func FrameNpcHtmlMessage(objectID int32, html string, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeNpcHtmlMessage)
	w.WriteInt32(objectID)
	w.WriteString(html)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
