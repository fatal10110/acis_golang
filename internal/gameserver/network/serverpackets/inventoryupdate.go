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
// changes already queued by the inventory. Each entry is written from the
// instance state its update carries, not from the instance as it is now.
func FrameInventoryUpdate(updates []itemcontainer.Update, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeInventoryUpdate)
	if err := writeInventoryUpdate(w, updates, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeInventoryUpdate(w *wire.Writer, updates []itemcontainer.Update, templates *item.Table) error {
	count, err := wire.Uint16Count(len(updates))
	if err != nil {
		return err
	}
	w.WriteUint16(count)
	for _, update := range updates {
		w.WriteUint16(uint16(update.State))
		if err := writeUpdatedItem(w, update, templates); err != nil {
			return fmt.Errorf("serverpackets: InventoryUpdate: %w", err)
		}
		w.WriteInt32(update.Augmentation)
		// The displayed mana is the queued mana in minutes, also for a
		// regular item, whose -1 shows as 0.
		w.WriteInt32(int32(update.ManaLeft / 60))
	}
	return nil
}

// writeUpdatedItem writes the item fields an inventory update entry shares
// between the player and the pet packet, from update alone.
func writeUpdatedItem(w *wire.Writer, update itemcontainer.Update, templates *item.Table) error {
	tmpl, ok := templates.Get(update.TemplateID)
	if !ok {
		return fmt.Errorf("no template loaded for item template %d", update.TemplateID)
	}
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
	return nil
}
