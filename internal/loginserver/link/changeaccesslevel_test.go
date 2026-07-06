package link

import (
	"encoding/binary"
	"testing"
)

func TestDecodeChangeAccessLevel(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeChangeAccessLevel}, 100)
	payload = appendString(payload, "alice")

	got, err := DecodeChangeAccessLevel(payload)
	if err != nil {
		t.Fatalf("DecodeChangeAccessLevel: %v", err)
	}
	want := ChangeAccessLevel{Level: 100, Account: "alice"}
	if got != want {
		t.Fatalf("DecodeChangeAccessLevel() = %+v, want %+v", got, want)
	}
}

func TestDecodeChangeAccessLevelShort(t *testing.T) {
	if _, err := DecodeChangeAccessLevel([]byte{OpcodeChangeAccessLevel, 1, 2}); err == nil {
		t.Error("DecodeChangeAccessLevel: want error on short payload, got nil")
	}
}

func TestEncodeChangeAccessLevelRoundTrip(t *testing.T) {
	want := ChangeAccessLevel{Level: -1, Account: "alice"}
	got, err := DecodeChangeAccessLevel(EncodeChangeAccessLevel(want))
	if err != nil {
		t.Fatalf("DecodeChangeAccessLevel(EncodeChangeAccessLevel()): %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}
