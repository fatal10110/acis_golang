package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeRequestUnEquipItem is the wire opcode for unequipping the item
// occupying a given body slot.
const OpcodeRequestUnEquipItem = 0x11

const unequipItemSize = 4

// UnequipItem requests unequipping whatever item occupies BodySlot, a
// Slot bitmask value from the item's own template.
type UnequipItem struct {
	BodySlot int32
}

// DecodeUnequipItem parses a raw UnequipItem payload (opcode byte
// included).
func DecodeUnequipItem(payload []byte) (UnequipItem, error) {
	r := newReader(payload)
	if r.Remaining() < unequipItemSize {
		return UnequipItem{}, fmt.Errorf("clientpackets: UnequipItem: need %d bytes, got %d: %w", unequipItemSize, r.Remaining(), wire.ErrShortPacket)
	}
	req := UnequipItem{BodySlot: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return UnequipItem{}, fmt.Errorf("clientpackets: UnequipItem: %w", err)
	}
	return req, nil
}
