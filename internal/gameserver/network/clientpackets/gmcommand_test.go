package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestDecodeRequestGMCommand reads the target name, then the view to show;
// a payload cut before the view's word is refused.
func TestDecodeRequestGMCommand(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestGMCommand)
	w.WriteString("Player")
	w.WriteInt32(6)

	got, err := DecodeRequestGMCommand(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestGMCommand: %v", err)
	}
	if want := (RequestGMCommand{TargetName: "Player", Command: 6}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	short := wire.NewPacketWriter(OpcodeRequestGMCommand)
	short.WriteString("Player")
	short.WriteUint16(6)
	if _, err := DecodeRequestGMCommand(short.Bytes()); err == nil {
		t.Fatal("DecodeRequestGMCommand: want error on a cut command word")
	}
	if _, err := DecodeRequestGMCommand([]byte{OpcodeRequestGMCommand, 'x'}); err == nil {
		t.Fatal("DecodeRequestGMCommand: want error on unterminated name")
	}
}
