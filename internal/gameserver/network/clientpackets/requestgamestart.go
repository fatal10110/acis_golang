package clientpackets

import "fmt"

// OpcodeRequestGameStart is the wire opcode for RequestGameStart, valid once
// a client is authenticated; it picks the character slot to enter the world
// with.
const OpcodeRequestGameStart = 0x0d

// requestGameStartSize is the slot field plus three trailing fields the
// client always sends but this server has no use for.
const requestGameStartSize = 4 + 2 + 4 + 4 + 4

// RequestGameStart asks to enter the world with the character in the given
// character-list slot.
type RequestGameStart struct {
	Slot int32
}

// DecodeRequestGameStart parses a raw RequestGameStart payload (opcode byte
// included). The trailing fields after the slot are read and discarded.
func DecodeRequestGameStart(payload []byte) (RequestGameStart, error) {
	r := newReader(payload)
	if r.Remaining() < requestGameStartSize {
		return RequestGameStart{}, fmt.Errorf("clientpackets: RequestGameStart: need %d bytes, got %d", requestGameStartSize, r.Remaining())
	}
	slot := r.ReadInt32()
	r.ReadInt16()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	return RequestGameStart{Slot: slot}, nil
}
