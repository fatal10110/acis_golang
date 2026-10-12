package serverpackets

import (
	"bytes"
	"testing"
)

// SetSummonRemainTime is opcode 0xd1, then the total and the remaining
// lifetime as int32s; a lifetime past its end goes out negative.
func TestFrameSetSummonRemainTime(t *testing.T) {
	got := framePayload(t, FrameSetSummonRemainTime(1200000, -200))
	want := []byte{0xd1, 0x80, 0x4f, 0x12, 0x00, 0x38, 0xff, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSetSummonRemainTime() = %x, want %x", got, want)
	}
}
