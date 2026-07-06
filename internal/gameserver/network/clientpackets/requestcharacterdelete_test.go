package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRequestCharacterDelete(t *testing.T) {
	payload := make([]byte, 1+requestCharacterDeleteSize)
	payload[0] = OpcodeRequestCharacterDelete
	binary.LittleEndian.PutUint32(payload[1:], 2)

	got, err := DecodeRequestCharacterDelete(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCharacterDelete: %v", err)
	}
	if want := (RequestCharacterDelete{Slot: 2}); got != want {
		t.Errorf("DecodeRequestCharacterDelete = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestCharacterDelete_Short(t *testing.T) {
	if _, err := DecodeRequestCharacterDelete([]byte{OpcodeRequestCharacterDelete, 0, 1}); err == nil {
		t.Error("DecodeRequestCharacterDelete: want error on short payload, got nil")
	}
}
