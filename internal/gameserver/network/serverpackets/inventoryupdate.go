package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// OpcodeInventoryUpdate is the wire opcode for player inventory deltas.
const OpcodeInventoryUpdate = 0x27

// FrameInventoryUpdate builds the player InventoryUpdate packet for item
// changes already queued by the inventory. Each entry is written from the
// instance state its update carries, not from the instance as it is now. An
// entry whose template templates no longer holds (one a reload removed) is
// left out.
func FrameInventoryUpdate(updates []itemcontainer.Update, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeInventoryUpdate)
	if err := writeInventoryUpdate(w, updates, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeInventoryUpdate(w *wire.Writer, updates []itemcontainer.Update, templates *item.Table) error {
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
		w.WriteInt32(update.Augmentation)
		// The displayed mana is the queued mana in minutes, also for a
		// regular item, whose -1 shows as 0.
		w.WriteInt32(int32(update.ManaLeft / 60))
		written++
	}
	w.PatchUint16(countOffset, uint16(written))
	return nil
}

// writeUpdatedItem writes the item fields an inventory update entry shares
// between the player and the pet packet, from update and its template.
func writeUpdatedItem(w *wire.Writer, update itemcontainer.Update, tmpl *item.Template) {
	category, subCategory := tmpl.Category()
	w.WriteUint16(uint16(category))
	w.WriteInt32(update.ObjectID)
	w.WriteInt32(update.TemplateID)
	w.WriteInt32(int32(update.Count))
	w.WriteUint16(uint16(subCategory))
	w.WriteUint16(uint16(update.CustomType1))
	w.WriteUint16(uint16(boolInt32(update.Equipped)))
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(update.EnchantLevel))
	w.WriteUint16(uint16(update.CustomType2))
}

// reserveUint16Count checks that n rows fit a 16-bit count and writes a
// placeholder for it, returning the offset to PatchUint16 with the rows a
// list builder actually wrote, which a skipped row makes fewer than n.
func reserveUint16Count(w *wire.Writer, n int) (int, error) {
	if _, err := wire.Uint16Count(n); err != nil {
		return 0, err
	}
	offset := w.Len()
	w.WriteUint16(0)
	return offset, nil
}
