package clientpackets

import "fmt"

// OpcodeCharacterRestore is the wire opcode for CharacterRestore, valid once
// a client is authenticated.
const OpcodeCharacterRestore = 0x62

const characterRestoreSize = 4

// CharacterRestore asks to clear a scheduled deletion for the character in
// the given character-list slot.
type CharacterRestore struct {
	Slot int32
}

// DecodeCharacterRestore parses a raw CharacterRestore payload (opcode byte
// included).
func DecodeCharacterRestore(payload []byte) (CharacterRestore, error) {
	r := newReader(payload)
	if r.Remaining() < characterRestoreSize {
		return CharacterRestore{}, fmt.Errorf("clientpackets: CharacterRestore: need %d bytes, got %d", characterRestoreSize, r.Remaining())
	}
	return CharacterRestore{Slot: r.ReadInt32()}, nil
}
