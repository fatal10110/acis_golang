package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
)

// OpcodeSendMacroList is the wire opcode for SendMacroList.
const OpcodeSendMacroList = 0xe7

// FrameSendMacroList builds one SendMacroList packet: the list's revision,
// its macro count, and one macro, or none for the empty list's single
// packet. A list of n macros is sent as n packets sharing one revision.
func FrameSendMacroList(revision int32, count int, m *macro.Macro) wire.Frame {
	w := newFrameWriter(OpcodeSendMacroList)
	w.WriteInt32(revision)
	w.WriteUint8(0) // unknown
	w.WriteUint8(uint8(count))
	if m == nil {
		w.WriteUint8(0) // no macro follows
		return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
	}
	w.WriteUint8(1) // one macro follows
	w.WriteInt32(m.ID)
	w.WriteString(m.Name)
	w.WriteString(m.Description)
	w.WriteString(m.Acronym)
	w.WriteUint8(uint8(m.Icon))
	w.WriteUint8(uint8(len(m.Commands)))
	for i, c := range m.Commands {
		w.WriteUint8(uint8(i + 1)) // lines are renumbered from one
		w.WriteUint8(uint8(c.Type))
		w.WriteInt32(c.D1)
		w.WriteUint8(uint8(c.D2))
		w.WriteString(c.Text)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
