package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameConfirmDlgEngageRequest pins the marriage request's bytes:
// ConfirmDlg.writeImpl for new ConfirmDlg(1983).addString(text), one
// TYPE_TEXT entry with a zero time and requester id, which it leaves off
// the wire.
func TestFrameConfirmDlgEngageRequest(t *testing.T) {
	got := framePayload(t, FrameConfirmDlgEngageRequest("Al ?"))
	want := []byte{
		0xed,
		0xbf, 0x07, 0x00, 0x00, // 1983
		0x01, 0x00, 0x00, 0x00, // 1 info entry
		0x00, 0x00, 0x00, 0x00, // TYPE_TEXT
		'A', 0x00, 'l', 0x00, ' ', 0x00, '?', 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameConfirmDlgEngageRequest() = %x, want %x", got, want)
	}
}
