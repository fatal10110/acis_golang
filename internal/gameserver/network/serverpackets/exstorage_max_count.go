package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

const (
	warehouseSlotsNoDwarf  = 100
	warehouseSlotsDwarf    = 120
	freightSlots           = 20
	privateStoreSlots      = 4
	privateStoreSlotsDwarf = 5
	dwarfRecipeLimit       = 50
	commonRecipeLimit      = 50
)

// FrameExStorageMaxCount builds the storage-limit packet sent during world
// entry.
func FrameExStorageMaxCount(c *player.Character) wire.Frame {
	warehouseLimit := int32(warehouseSlotsNoDwarf)
	inventoryLimit := int32(nonDwarfInventoryLimit)
	privateLimit := int32(privateStoreSlots)
	if c != nil && c.Race == player.RaceDwarf {
		warehouseLimit = warehouseSlotsDwarf
		inventoryLimit = dwarfInventoryLimit
		privateLimit = privateStoreSlotsDwarf
	}

	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExStorageMaxCount)
	w.WriteInt32(inventoryLimit)
	w.WriteInt32(warehouseLimit)
	w.WriteInt32(freightSlots)
	w.WriteInt32(privateLimit)
	w.WriteInt32(privateLimit)
	w.WriteInt32(dwarfRecipeLimit)
	w.WriteInt32(commonRecipeLimit)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
