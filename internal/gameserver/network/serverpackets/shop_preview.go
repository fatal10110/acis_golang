package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	// OpcodeShopPreviewList is the wire opcode for a merchant's try-on
	// window.
	OpcodeShopPreviewList = 0xef
	// OpcodeShopPreviewInfo is the wire opcode showing items tried on.
	OpcodeShopPreviewInfo = 0xf0
)

// FrameShopPreviewList builds the try-on window of list: every equipable
// product of a grade within expertise (the Expertise skill level), each
// priced wearPrice (WearPrice).
func FrameShopPreviewList(list buylist.List, currentMoney, expertise, wearPrice int, templates *item.Table) (wire.Frame, error) {
	shown := make([]*item.Template, 0, len(list.Products))
	for _, p := range list.Products {
		tmpl, ok := templates.Get(p.ItemID)
		if !ok {
			return wire.Frame{}, fmt.Errorf("serverpackets: ShopPreviewList: no template loaded for item template %d", p.ItemID)
		}
		if int(tmpl.Crystal) <= expertise && tmpl.Equipable() {
			shown = append(shown, tmpl)
		}
	}
	n, err := wire.Uint16Count(len(shown))
	if err != nil {
		return wire.Frame{}, err
	}
	w := newFrameWriter(OpcodeShopPreviewList)
	w.WriteUint8(0xc0)
	w.WriteUint8(0x13)
	w.WriteUint8(0)
	w.WriteUint8(0)
	w.WriteInt32(int32(currentMoney))
	w.WriteInt32(int32(list.ID))
	w.WriteUint16(n)
	for _, tmpl := range shown {
		category, subCategory := tmpl.Category()
		w.WriteInt32(tmpl.ID)
		w.WriteUint16(uint16(subCategory))
		// The slot is a 16-bit field: the higher slot bits do not fit.
		slot := uint16(tmpl.Slot)
		if category == item.CategoryMoneyOrEtcItem {
			slot = 0
		}
		w.WriteUint16(slot)
		w.WriteInt32(int32(wearPrice))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// previewInfoOrder is the paperdoll position each ShopPreviewInfo field
// shows, in wire order.
var previewInfoOrder = [item.PaperdollSlots]int{2, 1, 3, 5, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 0}

// FrameShopPreviewInfo builds the packet showing the items of items, the
// item id tried on at each paperdoll position, worn.
func FrameShopPreviewInfo(items [item.PaperdollSlots]int32) wire.Frame {
	w := newFrameWriter(OpcodeShopPreviewInfo)
	w.WriteInt32(item.PaperdollSlots)
	for _, slot := range previewInfoOrder {
		w.WriteInt32(items[slot])
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
