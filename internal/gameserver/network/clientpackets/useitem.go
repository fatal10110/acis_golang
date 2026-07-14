package clientpackets

import "fmt"

// OpcodeUseItem is the wire opcode for using or toggling the equip state
// of an inventory item.
const OpcodeUseItem = 0x14

const useItemSize = 2 * 4

// UseItem requests using or toggling the equip state of an inventory item.
type UseItem struct {
	ObjectID    int32
	CtrlPressed bool
}

// DecodeUseItem parses a raw UseItem payload (opcode byte included).
func DecodeUseItem(payload []byte) (UseItem, error) {
	r := newReader(payload)
	if r.Remaining() < useItemSize {
		return UseItem{}, fmt.Errorf("clientpackets: UseItem: need %d bytes, got %d", useItemSize, r.Remaining())
	}
	req := UseItem{
		ObjectID:    r.ReadInt32(),
		CtrlPressed: r.ReadInt32() != 0,
	}
	if err := r.Err(); err != nil {
		return UseItem{}, fmt.Errorf("clientpackets: UseItem: %w", err)
	}
	return req, nil
}
