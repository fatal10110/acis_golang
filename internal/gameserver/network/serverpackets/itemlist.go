package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeItemList is the wire opcode for ItemList, the full inventory sent on
// world entry and after any load that replaces the client's item state
// wholesale.
const OpcodeItemList = 0x1b

// FrameItemList builds the ItemList packet for everything a character is
// carrying: inventory items and, marked equipped, whatever sits on the
// paperdoll. Items in any other container (warehouse, freight, a pet's own
// hold) are a different list and don't belong here. templates must have an
// entry for every item's template id; a carried item with no loaded
// template is reported as an error rather than encoded around. On error no
// frame is returned and nothing needs releasing.
func FrameItemList(items []*item.Instance, templates *item.Table, showWindow bool) (wire.Frame, error) {
	w := newFrameWriter(OpcodeItemList)
	if err := writeItemList(w, items, templates, showWindow); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeItemList(w *wire.Writer, items []*item.Instance, templates *item.Table, showWindow bool) error {
	w.WriteUint16(uint16(boolInt32(showWindow)))
	countOffset := w.Len()
	w.WriteUint16(0) // backfilled with the owned count once the loop below finishes

	var owned int
	for _, raw := range items {
		it := raw.Snapshot()
		if it.Location != item.LocationInventory && it.Location != item.LocationPaperdoll {
			continue
		}

		tmpl, ok := templates.Get(it.TemplateID)
		if !ok {
			return fmt.Errorf("serverpackets: ItemList: no template loaded for item template %d", it.TemplateID)
		}
		category, subCategory := tmpl.Category()

		w.WriteUint16(uint16(category))
		w.WriteInt32(it.ObjectID)
		w.WriteInt32(it.TemplateID)
		w.WriteInt32(int32(it.Count))
		w.WriteUint16(uint16(subCategory))
		w.WriteUint16(uint16(it.CustomType1))
		w.WriteUint16(uint16(boolInt32(it.Location == item.LocationPaperdoll)))
		w.WriteInt32(int32(tmpl.Slot))
		w.WriteUint16(uint16(it.EnchantLevel))
		w.WriteUint16(uint16(it.CustomType2))
		if it.Augmentation != nil {
			w.WriteInt32(it.Augmentation.Attributes)
		} else {
			w.WriteInt32(0)
		}
		w.WriteInt32(int32(raw.DisplayedManaLeft(tmpl)))
		owned++
	}

	count, err := wire.Uint16Count(owned)
	if err != nil {
		return err
	}
	w.PatchUint16(countOffset, count)
	return nil
}
