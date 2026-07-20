package clientpackets

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRequestRestartPoint(t *testing.T) {
	payload := make([]byte, 1+requestRestartPointSize)
	payload[0] = OpcodeRequestRestartPoint
	binary.LittleEndian.PutUint32(payload[1:], 27)

	got, err := DecodeRequestRestartPoint(payload)
	if err != nil {
		t.Fatalf("DecodeRequestRestartPoint: %v", err)
	}
	if want := (RequestRestartPoint{RequestType: 27}); got != want {
		t.Errorf("DecodeRequestRestartPoint = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestRestartPoint_Short(t *testing.T) {
	if _, err := DecodeRequestRestartPoint([]byte{OpcodeRequestRestartPoint}); err == nil {
		t.Error("DecodeRequestRestartPoint: want error on short payload, got nil")
	}
}
