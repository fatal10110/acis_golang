package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// FrameExStorageMaxCount builds the storage-limit packet: c's live
// inventory, warehouse, freight, private sell/buy store and dwarven/common
// recipe limits, each its configured base plus its limit stat. A nil c
// reports the shipped non-dwarf defaults.
func FrameExStorageMaxCount(c *player.Character) wire.Frame {
	if c == nil {
		c = &player.Character{}
	}
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExStorageMaxCount)
	w.WriteInt32(clampInt32(c.InventoryLimit()))
	w.WriteInt32(clampInt32(c.WarehouseLimit()))
	w.WriteInt32(clampInt32(c.FreightLimit()))
	w.WriteInt32(clampInt32(c.PrivateSellStoreLimit()))
	w.WriteInt32(clampInt32(c.PrivateBuyStoreLimit()))
	w.WriteInt32(clampInt32(c.DwarfRecipeLimit()))
	w.WriteInt32(clampInt32(c.CommonRecipeLimit()))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
