package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeCharacterRestore(t *testing.T) {
	payload := make([]byte, 1+characterRestoreSize)
	payload[0] = OpcodeCharacterRestore
	binary.LittleEndian.PutUint32(payload[1:], 4)

	got, err := DecodeCharacterRestore(payload)
	if err != nil {
		t.Fatalf("DecodeCharacterRestore: %v", err)
	}
	if want := (CharacterRestore{Slot: 4}); got != want {
		t.Errorf("DecodeCharacterRestore = %+v, want %+v", got, want)
	}
}

func TestDecodeCharacterRestore_Short(t *testing.T) {
	if _, err := DecodeCharacterRestore([]byte{OpcodeCharacterRestore}); err == nil {
		t.Error("DecodeCharacterRestore: want error on short payload, got nil")
	}
}
