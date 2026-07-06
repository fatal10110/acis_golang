package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRequestGameStart(t *testing.T) {
	var payload []byte
	payload = append(payload, OpcodeRequestGameStart)
	payload = binary.LittleEndian.AppendUint32(payload, 2) // slot
	payload = binary.LittleEndian.AppendUint16(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored

	got, err := DecodeRequestGameStart(payload)
	if err != nil {
		t.Fatalf("DecodeRequestGameStart: %v", err)
	}
	if want := (RequestGameStart{Slot: 2}); got != want {
		t.Errorf("DecodeRequestGameStart = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestGameStart_Short(t *testing.T) {
	if _, err := DecodeRequestGameStart([]byte{OpcodeRequestGameStart, 0, 1}); err == nil {
		t.Error("DecodeRequestGameStart: want error on short payload, got nil")
	}
}
