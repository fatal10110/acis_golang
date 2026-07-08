package link

import (
	"bytes"
	"testing"
)

func TestEncodeKickPlayer(t *testing.T) {
	got := EncodeKickPlayer("alice")
	want := appendString([]byte{OpcodeKickPlayer}, "alice")
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeKickPlayer() = %x, want %x", got, want)
	}
}

func TestDecodeKickPlayer(t *testing.T) {
	got, err := DecodeKickPlayer(EncodeKickPlayer("alice"))
	if err != nil {
		t.Fatalf("DecodeKickPlayer: %v", err)
	}
	if got != "alice" {
		t.Fatalf("DecodeKickPlayer() = %q, want %q", got, "alice")
	}
}

func TestDecodeKickPlayerShort(t *testing.T) {
	if _, err := DecodeKickPlayer([]byte{OpcodeKickPlayer, 'a'}); err == nil {
		t.Error("DecodeKickPlayer: want error on unterminated string, got nil")
	}
}
