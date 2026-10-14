package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameConfirmDlgGate pins a clan hall door's question: new
// ConfirmDlg(1140) or ConfirmDlg(1141) with no info entry, so the entry
// count, the time and the requester id are all written as zero.
func TestFrameConfirmDlgGate(t *testing.T) {
	for _, tc := range []struct {
		open bool
		id   byte
	}{{open: true, id: 0x74}, {open: false, id: 0x75}} {
		got := framePayload(t, FrameConfirmDlgGate(tc.open))
		want := []byte{
			0xed,
			tc.id, 0x04, 0x00, 0x00, // 1140 or 1141
			0x00, 0x00, 0x00, 0x00, // no info entry
			0x00, 0x00, 0x00, 0x00, // time
			0x00, 0x00, 0x00, 0x00, // requester id
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("FrameConfirmDlgGate(%v) = %x, want %x", tc.open, got, want)
		}
	}
}
