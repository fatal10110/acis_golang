package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// OpcodeInventoryUpdate is the wire opcode for player inventory deltas.
const OpcodeInventoryUpdate = 0x27

// FrameInventoryUpdate builds the player InventoryUpdate packet for item
// changes already queued by the inventory.
func FrameInventoryUpdate(updates []itemcontainer.Update, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeInventoryUpdate)
	if err := writeInventoryUpdate(w, updates, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeInventoryUpdate(w *wire.Writer, updates []itemcontainer.Update, items []*item.Instance, templates *item.Table) error {
	byObjectID := make(map[int32]*item.Instance, len(items))
	for _, inst := range items {
		if inst != nil {
			byObjectID[inst.ObjectID] = inst
		}
	}

	w.WriteUint16(uint16(len(updates)))
	for _, update := range updates {
		inst := byObjectID[update.ObjectID]
		templateID := update.TemplateID
		count := update.Count
		var st item.InstanceState
		if inst != nil {
			st = inst.Snapshot()
			templateID = st.TemplateID
			if count == 0 {
				count = st.Count
			}
		}

		tmpl, ok := templates.Get(templateID)
		if !ok {
			return fmt.Errorf("serverpackets: InventoryUpdate: no template loaded for item template %d", templateID)
		}
		category, subCategory := tmpl.Category()

		w.WriteUint16(uint16(update.State))
		w.WriteUint16(uint16(category))
		w.WriteInt32(update.ObjectID)
		w.WriteInt32(templateID)
		w.WriteInt32(int32(count))
		w.WriteUint16(uint16(subCategory))
		if inst == nil {
			w.WriteUint16(0)
			w.WriteUint16(0)
			w.WriteInt32(int32(tmpl.Slot))
			w.WriteUint16(0)
			w.WriteUint16(0)
			w.WriteInt32(0)
			w.WriteInt32(-1)
			continue
		}
		w.WriteUint16(uint16(st.CustomType1))
		w.WriteUint16(uint16(boolInt32(st.Location == item.LocationPaperdoll)))
		w.WriteInt32(int32(tmpl.Slot))
		w.WriteUint16(uint16(st.EnchantLevel))
		w.WriteUint16(uint16(st.CustomType2))
		if st.Augmentation != nil {
			w.WriteInt32(st.Augmentation.Attributes)
		} else {
			w.WriteInt32(0)
		}
		w.WriteInt32(int32(inst.DisplayedManaLeft(tmpl)))
	}
	return nil
}
