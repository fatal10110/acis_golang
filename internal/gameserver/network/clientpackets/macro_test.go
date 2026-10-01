package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestDecodeRequestMakeMacroReadsAtMostTwelveLines pins the reference read
// (RequestMakeMacro.readImpl): id, name, description, acronym, icon byte and
// count byte, then a count capped at 12 of entry, type, d1 int32, d2 and
// text. A thirteenth line's bytes are left unread, not decoded.
func TestDecodeRequestMakeMacroReadsAtMostTwelveLines(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestMakeMacro)
	w.WriteInt32(1003)
	w.WriteString("Buffs")
	w.WriteString("desc")
	w.WriteString("BF")
	w.WriteUint8(200)
	w.WriteUint8(13)
	for i := range 13 {
		w.WriteUint8(uint8(i + 1))
		w.WriteUint8(3)
		w.WriteInt32(int32(100 + i))
		w.WriteUint8(uint8(i))
		w.WriteString("/x")
	}
	req, err := DecodeRequestMakeMacro(w.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.ID != 1003 || req.Name != "Buffs" || req.Description != "desc" || req.Acronym != "BF" || req.Icon != 200 {
		t.Fatalf("header = %+v", req)
	}
	if len(req.Commands) != 12 {
		t.Fatalf("decoded %d lines, want 12", len(req.Commands))
	}
	if last := req.Commands[11]; last.Type != 3 || last.D1 != 111 || last.D2 != 11 || last.Text != "/x" {
		t.Fatalf("line 12 = %+v", last)
	}
}

func TestDecodeRequestMakeMacroTruncatedLineIsShort(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestMakeMacro)
	w.WriteInt32(0)
	w.WriteString("A")
	w.WriteString("")
	w.WriteString("")
	w.WriteUint8(0)
	w.WriteUint8(1)
	w.WriteUint8(1)
	if _, err := DecodeRequestMakeMacro(w.Bytes()); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("err = %v, want ErrShortPacket", err)
	}
}
