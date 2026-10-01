package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeMultiSellChoose is the wire opcode of MultiSellChoose.
const OpcodeMultiSellChoose = 0xa7

// MultiSellChoose asks to exchange Amount units of entry EntryID (1-based)
// of the open multisell list ListID.
type MultiSellChoose struct {
	ListID  int32
	EntryID int32
	Amount  int32
}

// DecodeMultiSellChoose parses a raw MultiSellChoose payload (opcode byte
// included).
func DecodeMultiSellChoose(payload []byte) (MultiSellChoose, error) {
	r := newReader(payload)
	if r.Remaining() < 12 {
		return MultiSellChoose{}, fmt.Errorf("clientpackets: MultiSellChoose: need 12 bytes, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	req := MultiSellChoose{ListID: r.ReadInt32(), EntryID: r.ReadInt32(), Amount: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return MultiSellChoose{}, fmt.Errorf("clientpackets: MultiSellChoose: %w", err)
	}
	return req, nil
}
