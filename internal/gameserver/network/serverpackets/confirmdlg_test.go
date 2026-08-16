package serverpackets

import (
	"bytes"
	"testing"
	"time"
)

func TestFrameConfirmDlgSummonFriendRequest(t *testing.T) {
	got := framePayload(t, FrameConfirmDlgSummonFriendRequest("Bob", 12345, 10, 20, 30, 30*time.Second))
	want := []byte{
		OpcodeConfirmDlg,
		0x32, 0x07, 0x00, 0x00, // 1842
		0x02, 0x00, 0x00, 0x00, // 2 info entries
		confirmDlgTypeText, 0x00, 0x00, 0x00,
		'B', 0x00, 'o', 0x00, 'b', 0x00, 0x00, 0x00,
		confirmDlgTypeZoneName, 0x00, 0x00, 0x00,
		10, 0x00, 0x00, 0x00,
		20, 0x00, 0x00, 0x00,
		30, 0x00, 0x00, 0x00,
		0x30, 0x75, 0x00, 0x00, // 30000ms
		0x39, 0x30, 0x00, 0x00, // 12345
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameConfirmDlgSummonFriendRequest() = %x, want %x", got, want)
	}
}
