package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// TestFrameExShowQuestMark pins the quest mark's bytes, opcode 0xfe with
// sub-opcode 0x1a then the quest id, and its rendering against the line
// the engine-contract goldens write for it.
func TestFrameExShowQuestMark(t *testing.T) {
	got := framePayload(t, FrameExShowQuestMark(258))
	want := []byte{0xfe, 0x1a, 0x00, 0x02, 0x01, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowQuestMark(258) = %x, want %x", got, want)
	}
	line, err := scriptcontract.Packet(got, nil)
	if err != nil || line != "S ExShowQuestMark quest=258" {
		t.Fatalf("rendered = %q, %v", line, err)
	}
}
