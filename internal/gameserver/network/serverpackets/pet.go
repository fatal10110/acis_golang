package serverpackets

import (
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
	// OpcodePetItemList is the wire opcode for a full pet inventory snapshot.
	OpcodePetItemList = 0xb2
	// OpcodePetDelete tells the owner to remove a pet/servitor object from
	// the client.
	OpcodePetDelete = 0xb6
	// OpcodeSetSummonRemainTime is the wire opcode for a servitor's
	// remaining-lifetime bar.
	OpcodeSetSummonRemainTime = 0xd1
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

// FrameSetSummonRemainTime builds a SetSummonRemainTime packet: the
// servitor's total and remaining lifetime, in ms.
func FrameSetSummonRemainTime(total, remaining int32) wire.Frame {
	w := newFrameWriter(OpcodeSetSummonRemainTime)
	w.WriteInt32(total)
	w.WriteInt32(remaining)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePetItemList builds a full pet inventory snapshot. An item whose
// template templates no longer holds (one a reload removed) is left out.
func FramePetItemList(items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePetItemList)
	if err := writePetItemList(w, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writePetItemList(w *wire.Writer, items []*item.Instance, templates *item.Table) error {
	countOffset, err := reserveUint16Count(w, len(items))
	if err != nil {
		return err
	}
	var written int
	for _, inst := range items {
		if inst == nil {
			continue
		}
		tmpl, ok := templates.Get(inst.TemplateID)
		if !ok {
			continue
		}
		writeUpdatedItem(w, itemcontainer.NewUpdate(inst.Snapshot(), 0), tmpl)
		written++
	}
	w.PatchUint16(countOffset, uint16(written))
	return nil
}

// FramePetInventoryUpdate builds a PetInventoryUpdate packet for queued pet
// inventory changes, each entry written from the instance state its update
// carries. An entry whose template templates no longer holds is left out.
func FramePetInventoryUpdate(updates []itemcontainer.Update, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePetInventoryUpdate)
	if err := writePetInventoryUpdate(w, updates, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writePetInventoryUpdate(w *wire.Writer, updates []itemcontainer.Update, templates *item.Table) error {
	countOffset, err := reserveUint16Count(w, len(updates))
	if err != nil {
		return err
	}
	var written int
	for _, update := range updates {
		tmpl, ok := templates.Get(update.TemplateID)
		if !ok {
			continue
		}
		w.WriteUint16(uint16(update.State))
		writeUpdatedItem(w, update, tmpl)
		written++
	}
	w.PatchUint16(countOffset, uint16(written))
	return nil
}
