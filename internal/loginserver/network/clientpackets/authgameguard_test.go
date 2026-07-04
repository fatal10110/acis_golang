package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeAuthGameGuard(t *testing.T) {
	payload := make([]byte, 1+authGameGuardSize)
	payload[0] = OpcodeAuthGameGuard
	binary.LittleEndian.PutUint32(payload[1:], 0x11223344)
	binary.LittleEndian.PutUint32(payload[5:], 1)
	binary.LittleEndian.PutUint32(payload[9:], 2)
	binary.LittleEndian.PutUint32(payload[13:], 3)
	binary.LittleEndian.PutUint32(payload[17:], 4)

	got, err := DecodeAuthGameGuard(payload)
	if err != nil {
		t.Fatalf("DecodeAuthGameGuard: %v", err)
	}
	want := AuthGameGuard{SessionID: 0x11223344, Data1: 1, Data2: 2, Data3: 3, Data4: 4}
	if got != want {
		t.Errorf("DecodeAuthGameGuard = %+v, want %+v", got, want)
	}
}

func TestDecodeAuthGameGuardShort(t *testing.T) {
	if _, err := DecodeAuthGameGuard(make([]byte, 10)); err == nil {
		t.Error("DecodeAuthGameGuard: want error on short payload, got nil")
	}
}
