package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeItemList is the wire opcode for ItemList, the full inventory sent on
// world entry and after any load that replaces the client's item state
// wholesale.
const OpcodeItemList = 0x1b

// EncodeItemList builds the ItemList packet for everything a character is
// carrying: inventory items and, marked equipped, whatever sits on the
// paperdoll. Items in any other container (warehouse, freight, a pet's own
// hold) are a different list and don't belong here. templates must have an
// entry for every item's template id; a carried item with no loaded
// template is reported as an error rather than encoded around.
func EncodeItemList(items []*item.Instance, templates *item.Table, showWindow bool) ([]byte, error) {
	owned := make([]*item.Instance, 0, len(items))
	for _, it := range items {
		if it.Location == item.LocationInventory || it.Location == item.LocationPaperdoll {
			owned = append(owned, it)
		}
	}

	w := newWriter(OpcodeItemList)
	w.WriteInt16(uint16(boolInt32(showWindow)))
	w.WriteInt16(uint16(len(owned)))

	for _, it := range owned {
		tmpl, ok := templates.Get(it.TemplateID)
		if !ok {
			return nil, fmt.Errorf("serverpackets: EncodeItemList: no template loaded for item template %d", it.TemplateID)
		}
		category, subCategory := tmpl.Category()

		w.WriteInt16(uint16(category))
		w.WriteInt32(it.ObjectID)
		w.WriteInt32(it.TemplateID)
		w.WriteInt32(int32(it.Count))
		w.WriteInt16(uint16(subCategory))
		w.WriteInt16(uint16(it.CustomType1))
		w.WriteInt16(uint16(boolInt32(it.Location == item.LocationPaperdoll)))
		w.WriteInt32(int32(tmpl.Slot))
		w.WriteInt16(uint16(it.EnchantLevel))
		w.WriteInt16(uint16(it.CustomType2))
		w.WriteInt32(0)  // augmentation id: item augmentation is not modeled
		w.WriteInt32(-1) // displayed mana left: shadow-item duration is not modeled
	}
	return w.Bytes(), nil
}
