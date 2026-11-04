package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
)

// Opcodes of the equipped-dye summaries.
const (
	OpcodeHennaInfo   = 0xe4
	OpcodeGMHennaInfo = 0xea
)

// FrameHennaInfo builds the equipped-dye summary for HennaInfo (0xe4).
func FrameHennaInfo(snap henna.Snapshot) wire.Frame {
	return frameHennaSummary(OpcodeHennaInfo, snap)
}

// FrameGMHennaInfo builds a game master's view of a character's equipped
// dyes: HennaInfo's summary under its own opcode.
func FrameGMHennaInfo(snap henna.Snapshot) wire.Frame {
	return frameHennaSummary(OpcodeGMHennaInfo, snap)
}

func frameHennaSummary(opcode byte, snap henna.Snapshot) wire.Frame {
	w := newFrameWriter(opcode)
	w.WriteUint8(uint8(snap.INT))
	w.WriteUint8(uint8(snap.STR))
	w.WriteUint8(uint8(snap.CON))
	w.WriteUint8(uint8(snap.MEN))
	w.WriteUint8(uint8(snap.DEX))
	w.WriteUint8(uint8(snap.WIT))
	w.WriteInt32(int32(snap.MaxSlots))
	w.WriteInt32(int32(len(snap.Equipped)))
	for _, e := range snap.Equipped {
		w.WriteInt32(int32(e.SymbolID))
		w.WriteInt32(int32(e.ActiveSymbolID))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
