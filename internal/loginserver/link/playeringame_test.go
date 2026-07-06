package link

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestDecodePlayerInGame(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 2)
	payload = appendString(payload, "alice")
	payload = appendString(payload, "bob")

	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame: %v", err)
	}
	want := []string{"alice", "bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodePlayerInGame() = %v, want %v", got, want)
	}
}

func TestDecodePlayerInGameEmpty(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 0)
	got, err := DecodePlayerInGame(payload)
	if err != nil {
		t.Fatalf("DecodePlayerInGame: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DecodePlayerInGame() = %v, want empty", got)
	}
}

func TestDecodePlayerInGameShort(t *testing.T) {
	payload := binary.LittleEndian.AppendUint16([]byte{OpcodePlayerInGame}, 5)
	if _, err := DecodePlayerInGame(payload); err == nil {
		t.Error("DecodePlayerInGame: want error on truncated payload, got nil")
	}
}

func TestEncodePlayerInGameRoundTrip(t *testing.T) {
	want := []string{"alice", "bob"}
	got, err := DecodePlayerInGame(EncodePlayerInGame(want))
	if err != nil {
		t.Fatalf("DecodePlayerInGame(EncodePlayerInGame()): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

func TestEncodePlayerInGameSingle(t *testing.T) {
	want := []string{"alice"}
	got, err := DecodePlayerInGame(EncodePlayerInGame(want))
	if err != nil {
		t.Fatalf("DecodePlayerInGame(EncodePlayerInGame()): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}
