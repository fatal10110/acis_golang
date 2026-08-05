package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeRequestCharacterDelete is the wire opcode for RequestCharacterDelete,
// valid once a client is authenticated.
const OpcodeRequestCharacterDelete = 0x0c

const requestCharacterDeleteSize = 4

// RequestCharacterDelete asks to schedule the character in the given
// character-list slot for deletion.
type RequestCharacterDelete struct {
	Slot int32
}

// DecodeRequestCharacterDelete parses a raw RequestCharacterDelete payload
// (opcode byte included).
func DecodeRequestCharacterDelete(payload []byte) (RequestCharacterDelete, error) {
	r := newReader(payload)
	if r.Remaining() < requestCharacterDeleteSize {
		return RequestCharacterDelete{}, fmt.Errorf("clientpackets: RequestCharacterDelete: need %d bytes, got %d: %w", requestCharacterDeleteSize, r.Remaining(), wire.ErrShortPacket)
	}
	return RequestCharacterDelete{Slot: r.ReadInt32()}, nil
}
