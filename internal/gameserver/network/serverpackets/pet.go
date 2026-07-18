package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const (
	// OpcodePetStatusShow is the wire opcode for the small pet action/status
	// window toggle packet.
	OpcodePetStatusShow = 0xb0
	// OpcodePetInventoryUpdate is the wire opcode for pet inventory deltas.
	OpcodePetInventoryUpdate = 0xb3
	// OpcodePetDelete tells the owner to remove a pet/servitor object from
	// the client.
	OpcodePetDelete = 0xb6
)

// FramePetStatusShow builds a PetStatusShow packet for summonType.
func FramePetStatusShow(summonType int) wire.Frame {
	w := newFrameWriter(OpcodePetStatusShow)
	w.WriteInt32(int32(summonType))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePetDelete builds a PetDelete packet for summonType and objectID.
func FramePetDelete(summonType int, objectID int32) wire.Frame {
	w := newFrameWriter(OpcodePetDelete)
	w.WriteInt32(int32(summonType))
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePetInventoryUpdate builds a PetInventoryUpdate packet for queued pet
// inventory changes.
func FramePetInventoryUpdate(updates []itemcontainer.Update, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePetInventoryUpdate)
	if err := writePetInventoryUpdate(w, updates, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writePetInventoryUpdate(w *wire.Writer, updates []itemcontainer.Update, items []*item.Instance, templates *item.Table) error {
	byObjectID := make(map[int32]*item.Instance, len(items))
	for _, inst := range items {
		if inst != nil {
			byObjectID[inst.ObjectID] = inst
		}
	}

	w.WriteUint16(uint16(len(updates)))
	for _, update := range updates {
		inst := byObjectID[update.ObjectID]
		w.WriteUint16(uint16(update.State))
		if err := writePetItem(w, inst, templates, update); err != nil {
			return err
		}
	}
	return nil
}

func writePetItem(w *wire.Writer, inst *item.Instance, templates *item.Table, update itemcontainer.Update) error {
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
		return fmt.Errorf("serverpackets: pet item: no template loaded for item template %d", templateID)
	}
	category, subCategory := tmpl.Category()
	objectID := update.ObjectID
	if inst != nil {
		objectID = inst.ObjectID
	}

	w.WriteUint16(uint16(category))
	w.WriteInt32(objectID)
	w.WriteInt32(templateID)
	w.WriteInt32(int32(count))
	w.WriteUint16(uint16(subCategory))
	if inst == nil {
		w.WriteUint16(0)
		w.WriteUint16(0)
		w.WriteInt32(int32(tmpl.Slot))
		w.WriteUint16(0)
		w.WriteUint16(0)
		return nil
	}
	w.WriteUint16(uint16(st.CustomType1))
	w.WriteUint16(uint16(boolInt32(st.Location == item.LocationPetEquip)))
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(st.EnchantLevel))
	w.WriteUint16(uint16(st.CustomType2))
	return nil
}
