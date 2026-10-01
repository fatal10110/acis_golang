package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

// OpcodeMultiSellList is the wire opcode of MultiSellList, one page of a
// multisell window.
const OpcodeMultiSellList = 0xd0

// FrameMultiSellList builds the page of list starting at entry index: the
// list id, the 1-based page number, whether it is the last page, the page
// size, then up to multisell.PageSize entries numbered from 1 across the
// whole list. An entry carries its stackable flag and its products and
// ingredients: item id (2 bytes), a product's body part, the item's type2
// (65535 for an item without a template), count and enchant level.
func FrameMultiSellList(list *multisell.List, index int) wire.Frame {
	size := len(list.Entries) - index
	finished := size <= multisell.PageSize
	if !finished {
		size = multisell.PageSize
	}
	w := newFrameWriter(OpcodeMultiSellList)
	w.WriteInt32(list.ID)
	w.WriteInt32(int32(1 + index/multisell.PageSize))
	w.WriteInt32(boolInt32(finished))
	w.WriteInt32(multisell.PageSize)
	w.WriteInt32(int32(size))
	for i := index; i < index+size; i++ {
		e := list.Entries[i]
		w.WriteInt32(int32(i + 1))
		w.WriteInt32(0)
		w.WriteInt32(0)
		w.WriteUint8(wire.BoolByte(e.Stackable()))
		w.WriteUint16(uint16(len(e.Products)))
		w.WriteUint16(uint16(len(e.Ingredients)))
		for _, p := range e.Products {
			w.WriteUint16(uint16(p.ItemID))
			if tmpl := p.Template(); tmpl != nil {
				_, type2 := tmpl.Category()
				w.WriteInt32(int32(tmpl.Slot))
				w.WriteUint16(uint16(type2))
			} else {
				w.WriteInt32(0)
				w.WriteUint16(65535)
			}
			w.WriteInt32(int32(p.Count))
			w.WriteUint16(uint16(p.EnchantLevel))
			w.WriteInt32(0)
			w.WriteInt32(0)
		}
		for _, in := range e.Ingredients {
			w.WriteUint16(uint16(in.ItemID))
			if tmpl := in.Template(); tmpl != nil {
				_, type2 := tmpl.Category()
				w.WriteUint16(uint16(type2))
			} else {
				w.WriteUint16(65535)
			}
			w.WriteInt32(int32(in.Count))
			w.WriteUint16(uint16(in.EnchantLevel))
			w.WriteInt32(0)
			w.WriteInt32(0)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
