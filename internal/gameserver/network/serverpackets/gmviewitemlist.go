package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeGMViewItemList is the wire opcode for a game master's view of an
// inventory.
const OpcodeGMViewItemList = 0x94

// FrameGMViewItemList builds a game master's view of the inventory owner
// carries: owner's name and slot limit, then every item in items, worn ones
// marked equipped, in ItemList's item layout. An item whose template
// templates no longer holds (one a reload removed) is left out. On error no
// frame is returned and nothing needs releasing.
func FrameGMViewItemList(owner string, limit int, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeGMViewItemList)
	if err := writeGMViewItemList(w, owner, limit, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeGMViewItemList(w *wire.Writer, owner string, limit int, items []*item.Instance, templates *item.Table) error {
	w.WriteString(owner)
	w.WriteInt32(int32(limit))
	w.WriteUint16(1) // show the window
	countOffset, err := reserveUint16Count(w, len(items))
	if err != nil {
		return err
	}
	var listed int
	for _, raw := range items {
		it := raw.Snapshot()
		tmpl, ok := templates.Get(it.TemplateID)
		if !ok {
			continue
		}
		listed++
		category, subCategory := tmpl.Category()
		worn := it.Location == item.LocationPaperdoll || it.Location == item.LocationPetEquip
		w.WriteUint16(uint16(category))
		w.WriteInt32(it.ObjectID)
		w.WriteInt32(it.TemplateID)
		w.WriteInt32(int32(it.Count))
		w.WriteUint16(uint16(subCategory))
		w.WriteUint16(uint16(it.CustomType1))
		w.WriteUint16(uint16(boolInt32(worn)))
		w.WriteInt32(int32(tmpl.Slot))
		w.WriteUint16(uint16(it.EnchantLevel))
		w.WriteUint16(uint16(it.CustomType2))
		if it.Augmentation != nil {
			w.WriteInt32(it.Augmentation.Attributes)
		} else {
			w.WriteInt32(0)
		}
		w.WriteInt32(int32(raw.DisplayedManaLeft(tmpl)))
	}
	w.PatchUint16(countOffset, uint16(listed))
	return nil
}
