package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
)

// Symbol maker window opcodes.
const (
	OpcodeHennaEquipList       = 0xe2
	OpcodeHennaItemInfo        = 0xe3
	OpcodeHennaUnequipList     = 0xe5
	OpcodeHennaItemUnequipInfo = 0xe6
)

// Symbol maker system messages (no parameter).
const (
	SystemMessageSymbolAdded    = 877
	SystemMessageSymbolDeleted  = 878
	SystemMessageCantDrawSymbol = 899
	SystemMessageSymbolsFull    = 900
	SystemMessageSymbolNotFound = 901
)

// HennaStats are the six base attributes a symbol window compares a
// symbol's change against, as the player has them now.
type HennaStats struct {
	INT, STR, CON, MEN, DEX, WIT int
}

// FrameHennaEquipList builds HennaEquipList (0xe2), the draw window: the
// player's adena, how many symbols the class may wear, then per symbol its
// id, dye, the dyes a draw takes, the draw price and a constant 1.
func FrameHennaEquipList(adena, maxSlots int, hennas []henna.Henna) wire.Frame {
	w := newFrameWriter(OpcodeHennaEquipList)
	w.WriteInt32(int32(adena))
	w.WriteInt32(int32(maxSlots))
	w.WriteInt32(int32(len(hennas)))
	for _, h := range hennas {
		w.WriteInt32(int32(h.SymbolID))
		w.WriteInt32(h.DyeID)
		w.WriteInt32(henna.DrawAmount)
		w.WriteInt32(int32(h.DrawPrice))
		w.WriteInt32(1)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameHennaUnequipList builds HennaUnequipList (0xe5), the deletion
// window: the player's adena, its free symbol slots, then per worn symbol
// its id, dye, the dyes a deletion hands back, the deletion price and a
// constant 1.
func FrameHennaUnequipList(adena, emptySlots int, hennas []henna.Henna) wire.Frame {
	w := newFrameWriter(OpcodeHennaUnequipList)
	w.WriteInt32(int32(adena))
	w.WriteInt32(int32(emptySlots))
	w.WriteInt32(int32(len(hennas)))
	for _, h := range hennas {
		w.WriteInt32(int32(h.SymbolID))
		w.WriteInt32(h.DyeID)
		w.WriteInt32(henna.RemoveAmount)
		w.WriteInt32(int32(h.RemovePrice()))
		w.WriteInt32(1)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameHennaItemInfo builds HennaItemInfo (0xe3), one symbol's draw
// details: its id, dye, dyes taken, price, a constant 1, the player's
// adena, then INT, STR, CON, MEN, DEX and WIT each as the current value
// and, in one byte, the value once drawn.
func FrameHennaItemInfo(h henna.Henna, adena int, now HennaStats) wire.Frame {
	w := newFrameWriter(OpcodeHennaItemInfo)
	writeHennaDetails(w, h, henna.DrawAmount, h.DrawPrice, adena)
	writeHennaStats(w, now, HennaStats{INT: now.INT + h.INT, STR: now.STR + h.STR, CON: now.CON + h.CON, MEN: now.MEN + h.MEN, DEX: now.DEX + h.DEX, WIT: now.WIT + h.WIT})
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameHennaItemUnequipInfo builds HennaItemUnequipInfo (0xe6), one worn
// symbol's deletion details: its id, dye, dyes handed back, price, a
// constant 1, the player's adena, then INT, STR, CON, MEN, DEX and WIT each
// as the current value and, in one byte, the value once deleted.
func FrameHennaItemUnequipInfo(h henna.Henna, adena int, now HennaStats) wire.Frame {
	w := newFrameWriter(OpcodeHennaItemUnequipInfo)
	writeHennaDetails(w, h, henna.RemoveAmount, h.RemovePrice(), adena)
	writeHennaStats(w, now, HennaStats{INT: now.INT - h.INT, STR: now.STR - h.STR, CON: now.CON - h.CON, MEN: now.MEN - h.MEN, DEX: now.DEX - h.DEX, WIT: now.WIT - h.WIT})
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeHennaDetails(w *wire.Writer, h henna.Henna, dyes, price, adena int) {
	w.WriteInt32(int32(h.SymbolID))
	w.WriteInt32(h.DyeID)
	w.WriteInt32(int32(dyes))
	w.WriteInt32(int32(price))
	w.WriteInt32(1)
	w.WriteInt32(int32(adena))
}

// writeHennaStats writes each attribute's current value as an int32 and
// its changed value as its low byte.
func writeHennaStats(w *wire.Writer, now, after HennaStats) {
	for _, pair := range [6][2]int{
		{now.INT, after.INT},
		{now.STR, after.STR},
		{now.CON, after.CON},
		{now.MEN, after.MEN},
		{now.DEX, after.DEX},
		{now.WIT, after.WIT},
	} {
		w.WriteInt32(int32(pair[0]))
		w.WriteUint8(uint8(pair[1]))
	}
}
